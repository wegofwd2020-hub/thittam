package observability

import (
	"context"
	"time"

	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryMetricsInterceptor returns a gRPC unary interceptor that records
// request duration, count, and active requests in Prometheus metrics.
func UnaryMetricsInterceptor(m *Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		m.ActiveRequests.Inc()
		defer m.ActiveRequests.Dec()

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start).Seconds()

		code := grpcCode(err)
		m.RequestDuration.WithLabelValues(info.FullMethod, code).Observe(duration)
		m.RequestCounter.WithLabelValues(info.FullMethod, code).Inc()

		// Tenant-scoped counter for billing
		if tenantID, ok := tenant.IDFromContext(ctx); ok {
			m.TenantRequests.WithLabelValues(tenantID.String()).Inc()
		}

		return resp, err
	}
}

// StreamMetricsInterceptor returns a gRPC stream interceptor for metrics.
func StreamMetricsInterceptor(m *Metrics) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		m.ActiveRequests.Inc()
		defer m.ActiveRequests.Dec()

		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start).Seconds()

		code := grpcCode(err)
		m.RequestDuration.WithLabelValues(info.FullMethod, code).Observe(duration)
		m.RequestCounter.WithLabelValues(info.FullMethod, code).Inc()

		if tenantID, ok := tenant.IDFromContext(ss.Context()); ok {
			m.TenantRequests.WithLabelValues(tenantID.String()).Inc()
		}

		return err
	}
}

func grpcCode(err error) string {
	if err == nil {
		return "OK"
	}
	if st, ok := status.FromError(err); ok {
		return st.Code().String()
	}
	return "Unknown"
}
