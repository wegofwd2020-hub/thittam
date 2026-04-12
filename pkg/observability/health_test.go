package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthz_AlwaysOK(t *testing.T) {
	hs := NewHealthServer("test-service", 0)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	hs.handleHealthz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status HealthStatus
	require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
	assert.Equal(t, "ok", status.Status)
	assert.Equal(t, "test-service", status.Service)
}

func TestReadyz_AllCheckersPass(t *testing.T) {
	hs := NewHealthServer("test-service", 0)
	hs.RegisterChecker("postgres", HealthCheckerFunc(func(ctx context.Context) error {
		return nil
	}))
	hs.RegisterChecker("redis", HealthCheckerFunc(func(ctx context.Context) error {
		return nil
	}))

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	hs.handleReadyz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status HealthStatus
	require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
	assert.Equal(t, "ok", status.Status)
	assert.Equal(t, "ok", status.Checks["postgres"])
	assert.Equal(t, "ok", status.Checks["redis"])
}

func TestReadyz_CheckerFails_Returns503(t *testing.T) {
	hs := NewHealthServer("test-service", 0)
	hs.RegisterChecker("postgres", HealthCheckerFunc(func(ctx context.Context) error {
		return nil
	}))
	hs.RegisterChecker("redis", HealthCheckerFunc(func(ctx context.Context) error {
		return errors.New("connection refused")
	}))

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	hs.handleReadyz(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var status HealthStatus
	require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
	assert.Equal(t, "degraded", status.Status)
	assert.Equal(t, "ok", status.Checks["postgres"])
	assert.Equal(t, "connection refused", status.Checks["redis"])
}

func TestReadyz_NoCheckers(t *testing.T) {
	hs := NewHealthServer("test-service", 0)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	hs.handleReadyz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status HealthStatus
	require.NoError(t, json.NewDecoder(w.Body).Decode(&status))
	assert.Equal(t, "ok", status.Status)
}

func TestHealthCheckerFunc(t *testing.T) {
	called := false
	checker := HealthCheckerFunc(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := checker.CheckHealth(context.Background())
	require.NoError(t, err)
	assert.True(t, called)
}

// ─── Start / Stop lifecycle ───────────────────────────────────────────────────

func TestHealthServer_Start_Stop_Lifecycle(t *testing.T) {
	t.Parallel()
	// Port 0 → OS assigns any available ephemeral port.
	hs := NewHealthServer("lifecycle-test", 0)

	require.NoError(t, hs.Start())

	// Give the goroutine time to bind and start listening.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, hs.Stop(ctx))
}

func TestHealthServer_Stop_Unstarted_IsNoop(t *testing.T) {
	t.Parallel()
	hs := NewHealthServer("unstarted-test", 0)
	// httpServer is nil — Stop must return nil without panicking.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, hs.Stop(ctx))
}

func TestHealthServer_Stop_NilReceiver_IsNoop(t *testing.T) {
	t.Parallel()
	var hs *HealthServer
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, hs.Stop(ctx))
}
