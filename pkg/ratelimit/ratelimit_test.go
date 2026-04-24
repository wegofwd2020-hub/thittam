package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRDB(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestMiddleware_AllowsWithinLimit(t *testing.T) {
	t.Parallel()
	rdb, _ := newTestRDB(t)
	mw := Middleware(rdb, Config{Limit: 3, Window: time.Minute, KeyPrefix: "test"})
	h := mw(okHandler())

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "10.1.2.3:4567"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should be allowed", i+1)
	}
}

func TestMiddleware_BlocksAboveLimit(t *testing.T) {
	t.Parallel()
	rdb, _ := newTestRDB(t)
	mw := Middleware(rdb, Config{Limit: 2, Window: time.Minute, KeyPrefix: "test"})
	h := mw(okHandler())

	// 2 allowed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "10.1.2.3:4567"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// 3rd blocked
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.1.2.3:4567"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), "Too many requests")
}

func TestMiddleware_KeysByIP(t *testing.T) {
	t.Parallel()
	rdb, _ := newTestRDB(t)
	mw := Middleware(rdb, Config{Limit: 1, Window: time.Minute, KeyPrefix: "test"})
	h := mw(okHandler())

	// First IP exhausts its quota.
	req1 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req1.RemoteAddr = "10.1.2.3:4567"
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req2.RemoteAddr = "10.1.2.3:4567"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusTooManyRequests, rec2.Code)

	// Second IP gets its own bucket.
	req3 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req3.RemoteAddr = "10.9.9.9:1234"
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)
}

func TestMiddleware_FailsOpenWhenRedisDown(t *testing.T) {
	t.Parallel()
	rdb, mr := newTestRDB(t)
	mw := Middleware(rdb, Config{Limit: 1, Window: time.Minute, KeyPrefix: "test"})
	h := mw(okHandler())

	// Kill Redis before the request lands.
	mr.Close()

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.1.2.3:4567"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Fail-open: request goes through despite Redis being down.
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestClientIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "192.168.1.5:12345",
			want:       "192.168.1.5",
		},
		{
			name:       "CF-Connecting-IP wins",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"CF-Connecting-IP": "203.0.113.45",
				"X-Real-IP":        "198.51.100.7",
				"X-Forwarded-For":  "198.51.100.7, 10.0.0.1",
			},
			want: "203.0.113.45",
		},
		{
			name:       "X-Real-IP when no Cloudflare",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Real-IP":       "198.51.100.7",
				"X-Forwarded-For": "198.51.100.7, 10.0.0.1",
			},
			want: "198.51.100.7",
		},
		{
			name:       "XFF first value",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.10, 10.0.0.1"},
			want:       "203.0.113.10",
		},
		{
			name:       "XFF whitespace trimmed",
			remoteAddr: "10.0.0.1:12345",
			headers:    map[string]string{"X-Forwarded-For": "  203.0.113.10  , 10.0.0.1"},
			want:       "203.0.113.10",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			assert.Equal(t, tc.want, clientIP(req))
		})
	}
}

func TestMiddleware_ResetsAfterWindow(t *testing.T) {
	t.Parallel()
	rdb, mr := newTestRDB(t)
	mw := Middleware(rdb, Config{Limit: 1, Window: 30 * time.Second, KeyPrefix: "test"})
	h := mw(okHandler())

	req1 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req1.RemoteAddr = "10.1.2.3:4567"
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req2.RemoteAddr = "10.1.2.3:4567"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusTooManyRequests, rec2.Code)

	// Advance miniredis past the window boundary.
	mr.FastForward(31 * time.Second)

	req3 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req3.RemoteAddr = "10.1.2.3:4567"
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)
}
