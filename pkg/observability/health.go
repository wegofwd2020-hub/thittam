package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HealthChecker is implemented by each service to report its readiness.
type HealthChecker interface {
	// CheckHealth returns nil if the service is ready, or an error describing the problem.
	CheckHealth(ctx context.Context) error
}

// HealthCheckerFunc adapts a function to the HealthChecker interface.
type HealthCheckerFunc func(ctx context.Context) error

func (f HealthCheckerFunc) CheckHealth(ctx context.Context) error {
	return f(ctx)
}

// HealthStatus is the JSON response for health endpoints.
type HealthStatus struct {
	Status  string            `json:"status"` // "ok" or "degraded"
	Service string            `json:"service"`
	Checks  map[string]string `json:"checks,omitempty"` // component → "ok" or error message
}

// HealthServer serves /healthz, /readyz, and /metrics on a dedicated HTTP port.
type HealthServer struct {
	serviceName string
	port        int
	mu          sync.RWMutex
	checkers    map[string]HealthChecker
	httpServer  *http.Server // retained so Stop can call Shutdown
}

// NewHealthServer creates a health/metrics HTTP server.
func NewHealthServer(serviceName string, port int) *HealthServer {
	return &HealthServer{
		serviceName: serviceName,
		port:        port,
		checkers:    make(map[string]HealthChecker),
	}
}

// RegisterChecker adds a named health checker (e.g., "postgres", "redis").
func (h *HealthServer) RegisterChecker(name string, checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

// Start begins serving health and metrics endpoints. Non-blocking.
func (h *HealthServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)
	mux.Handle("/metrics", promhttp.Handler())

	h.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go h.httpServer.ListenAndServe() //nolint:errcheck // ErrServerClosed is expected on Stop
	return nil
}

// Stop gracefully shuts down the health/metrics HTTP server. Should be called
// during service shutdown before the gRPC server drains, so that Kubernetes
// stops routing traffic to this pod before in-flight RPCs are drained.
// A nil or unstarted HealthServer is a no-op.
func (h *HealthServer) Stop(ctx context.Context) error {
	if h == nil || h.httpServer == nil {
		return nil
	}
	return h.httpServer.Shutdown(ctx)
}

// handleHealthz is a simple liveness check — always returns 200 if the process is running.
func (h *HealthServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthStatus{
		Status:  "ok",
		Service: h.serviceName,
	})
}

// handleReadyz checks all registered health checkers. Returns 200 if all pass,
// 503 if any fail.
func (h *HealthServer) handleReadyz(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checkers := make(map[string]HealthChecker, len(h.checkers))
	for k, v := range h.checkers {
		checkers[k] = v
	}
	h.mu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := make(map[string]string, len(checkers))
	allOK := true

	for name, checker := range checkers {
		if err := checker.CheckHealth(ctx); err != nil {
			checks[name] = err.Error()
			allOK = false
		} else {
			checks[name] = "ok"
		}
	}

	status := HealthStatus{
		Service: h.serviceName,
		Checks:  checks,
	}

	w.Header().Set("Content-Type", "application/json")
	if allOK {
		status.Status = "ok"
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(status)
}
