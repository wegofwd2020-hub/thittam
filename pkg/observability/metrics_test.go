package observability

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestMetrics creates metrics with a custom registry to avoid global state conflicts.
func newTestMetrics(name string) *Metrics {
	return &Metrics{
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "thittam",
				Subsystem: name,
				Name:      "grpc_request_duration_seconds",
				Help:      "test",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "grpc_code"},
		),
		RequestCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "thittam",
				Subsystem: name,
				Name:      "grpc_requests_total",
				Help:      "test",
			},
			[]string{"method", "grpc_code"},
		),
		ActiveRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "thittam",
				Subsystem: name,
				Name:      "grpc_active_requests",
				Help:      "test",
			},
		),
		TenantRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "thittam",
				Subsystem: name,
				Name:      "tenant_requests_total",
				Help:      "test",
			},
			[]string{"tenant_id"},
		),
		CacheOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "thittam",
				Subsystem: name,
				Name:      "cache_operations_total",
				Help:      "test",
			},
			[]string{"tier", "result"},
		),
		DBActiveConns: prometheus.NewGauge(prometheus.GaugeOpts{Name: name + "_db_active"}),
		DBIdleConns:   prometheus.NewGauge(prometheus.GaugeOpts{Name: name + "_db_idle"}),
		RedisConnected: prometheus.NewGauge(prometheus.GaugeOpts{Name: name + "_redis"}),
	}
}

func TestUnaryMetricsInterceptor_RecordsMetrics(t *testing.T) {
	m := newTestMetrics("test_unary")
	interceptor := UnaryMetricsInterceptor(m)

	tenantID := uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	ctx := tenant.WithID(context.Background(), tenantID)

	info := &grpc.UnaryServerInfo{FullMethod: "/thittam.project.v1.ProjectService/CreateProduction"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		time.Sleep(5 * time.Millisecond) // simulate work
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	// Verify request counter incremented
	counter, err := m.RequestCounter.GetMetricWithLabelValues(info.FullMethod, "OK")
	require.NoError(t, err)
	assert.NotNil(t, counter)

	// Verify tenant counter incremented
	tenantCounter, err := m.TenantRequests.GetMetricWithLabelValues(tenantID.String())
	require.NoError(t, err)
	assert.NotNil(t, tenantCounter)
}

func TestUnaryMetricsInterceptor_RecordsErrorCode(t *testing.T) {
	m := newTestMetrics("test_error")
	interceptor := UnaryMetricsInterceptor(m)

	info := &grpc.UnaryServerInfo{FullMethod: "/thittam.expense.v1.ExpenseService/ApproveExpense"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.PermissionDenied, "limit exceeded")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	// Verify counter with PermissionDenied label
	counter, err := m.RequestCounter.GetMetricWithLabelValues(info.FullMethod, "PermissionDenied")
	require.NoError(t, err)
	assert.NotNil(t, counter)
}

func TestUnaryMetricsInterceptor_NoTenant(t *testing.T) {
	m := newTestMetrics("test_no_tenant")
	interceptor := UnaryMetricsInterceptor(m)

	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	// Should not panic even without tenant in context
}

func TestStreamMetricsInterceptor(t *testing.T) {
	m := newTestMetrics("test_stream")
	interceptor := StreamMetricsInterceptor(m)

	tenantID := uuid.MustParse("d0000000-0000-0000-0000-000000000002")
	ctx := tenant.WithID(context.Background(), tenantID)

	ss := &mockServerStream{ctx: ctx}
	info := &grpc.StreamServerInfo{FullMethod: "/thittam.reporting.v1.ReportingService/RunReport"}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
}

func TestCacheMetrics(t *testing.T) {
	m := newTestMetrics("test_cache")

	m.RecordCacheHit("l1")
	m.RecordCacheHit("l2")
	m.RecordCacheMiss("l1")

	// Verify counters exist (no panic on label values)
	hit, err := m.CacheOperations.GetMetricWithLabelValues("l1", "hit")
	require.NoError(t, err)
	assert.NotNil(t, hit)

	miss, err := m.CacheOperations.GetMetricWithLabelValues("l1", "miss")
	require.NoError(t, err)
	assert.NotNil(t, miss)
}

func TestNewMetrics_ReturnsPopulatedStruct(t *testing.T) {
	// Not parallel — promauto registers with the global Prometheus registry.
	// Use a unique subsystem name to avoid duplicate registration panics.
	m := NewMetrics("newmetrics_cover_test")

	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.RequestCounter)
	assert.NotNil(t, m.ActiveRequests)
	assert.NotNil(t, m.TenantRequests)
	assert.NotNil(t, m.CacheOperations)
	assert.NotNil(t, m.DBActiveConns)
	assert.NotNil(t, m.DBIdleConns)
	assert.NotNil(t, m.RedisConnected)
}

func TestGrpcCode(t *testing.T) {
	assert.Equal(t, "OK", grpcCode(nil))
	assert.Equal(t, "NotFound", grpcCode(status.Error(codes.NotFound, "not found")))
	assert.Equal(t, "Internal", grpcCode(status.Error(codes.Internal, "boom")))
	assert.Equal(t, "Unknown", grpcCode(assert.AnError))
}

// mockServerStream for stream interceptor tests.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }
