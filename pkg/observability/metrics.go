// Package observability provides Prometheus metrics, health checks, and
// gRPC interceptors for all Thittam services.
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metric collectors for a service.
type Metrics struct {
	// gRPC request metrics
	RequestDuration *prometheus.HistogramVec
	RequestCounter  *prometheus.CounterVec
	ActiveRequests  prometheus.Gauge

	// Tenant-scoped request counter (for usage billing)
	TenantRequests *prometheus.CounterVec

	// Cache metrics
	CacheOperations *prometheus.CounterVec

	// DB connection pool
	DBActiveConns prometheus.Gauge
	DBIdleConns   prometheus.Gauge

	// Redis connection
	RedisConnected prometheus.Gauge
}

// NewMetrics creates all metric collectors for a service, registered with reg.
// A nil reg means the global prometheus.DefaultRegisterer (production default);
// tests pass a fresh prometheus.NewRegistry() so repeated construction with the
// same service name does not panic on duplicate registration.
func NewMetrics(serviceName string, reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	f := promauto.With(reg)
	return &Metrics{
		RequestDuration: f.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "grpc_request_duration_seconds",
				Help:      "Duration of gRPC requests in seconds.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"method", "grpc_code"},
		),

		RequestCounter: f.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "grpc_requests_total",
				Help:      "Total number of gRPC requests.",
			},
			[]string{"method", "grpc_code"},
		),

		ActiveRequests: f.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "grpc_active_requests",
				Help:      "Number of in-flight gRPC requests.",
			},
		),

		TenantRequests: f.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "tenant_requests_total",
				Help:      "Total gRPC requests per tenant (for usage billing).",
			},
			[]string{"tenant_id"},
		),

		CacheOperations: f.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "cache_operations_total",
				Help:      "Cache operations by tier and result.",
			},
			[]string{"tier", "result"}, // tier: l1/l2, result: hit/miss
		),

		DBActiveConns: f.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "db_active_connections",
				Help:      "Number of active database connections.",
			},
		),

		DBIdleConns: f.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "db_idle_connections",
				Help:      "Number of idle database connections.",
			},
		),

		RedisConnected: f.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "thittam",
				Subsystem: serviceName,
				Name:      "redis_connected",
				Help:      "Whether Redis is connected (1) or not (0).",
			},
		),
	}
}

// RecordCacheHit records a cache hit for the given tier.
func (m *Metrics) RecordCacheHit(tier string) {
	m.CacheOperations.WithLabelValues(tier, "hit").Inc()
}

// RecordCacheMiss records a cache miss for the given tier.
func (m *Metrics) RecordCacheMiss(tier string) {
	m.CacheOperations.WithLabelValues(tier, "miss").Inc()
}
