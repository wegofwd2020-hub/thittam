package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// noopRegister registers no handlers; the mux 404s but the CORS/Wrap chain
// still runs, which is all these tests exercise.
func noopRegister(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error {
	return nil
}

func TestBuildGatewayHandler_CORSPreflight(t *testing.T) {
	h, err := buildGatewayHandler(context.Background(), GatewayConfig{
		ServiceName: "test", GRPCEndpoint: "localhost:0", Register: noopRegister,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/anything", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, "http://localhost:3100", rec.Header().Get("Access-Control-Allow-Origin"))
}

func preflightAllowHeaders(t *testing.T, projectHeader bool) string {
	t.Helper()
	h, err := buildGatewayHandler(context.Background(), GatewayConfig{
		ServiceName: "test", GRPCEndpoint: "localhost:0",
		ProjectHeader: projectHeader, Register: noopRegister,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/anything", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "x-project-id")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Header().Get("Access-Control-Allow-Headers")
}

func TestBuildGatewayHandler_ProjectHeaderToggle(t *testing.T) {
	// rs/cors echoes a requested header in Access-Control-Allow-Headers only
	// when it is in AllowedHeaders. Compare case-insensitively (rs/cors
	// canonicalizes header names).
	on := strings.ToLower(preflightAllowHeaders(t, true))
	assert.Contains(t, on, "x-project-id", "ProjectHeader:true must allow X-Project-Id")

	off := strings.ToLower(preflightAllowHeaders(t, false))
	assert.NotContains(t, off, "x-project-id", "ProjectHeader:false must not allow X-Project-Id")
}

func TestBuildGatewayHandler_WrapInvoked(t *testing.T) {
	h, err := buildGatewayHandler(context.Background(), GatewayConfig{
		ServiceName: "test", GRPCEndpoint: "localhost:0", Register: noopRegister,
		Wrap: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Wrap-Marker", "1")
				next.ServeHTTP(w, r)
			})
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, "1", rec.Header().Get("X-Wrap-Marker"), "Wrap middleware must run inside CORS")
}
