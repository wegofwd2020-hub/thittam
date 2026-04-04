package vertical

import (
	"context"

	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryInterceptor returns a gRPC unary interceptor that loads the tenant's
// vertical config and injects it into the request context.
// Must be chained AFTER the auth interceptor (which sets the tenant ID).
func UnaryInterceptor(loader *Loader) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		ctx, err := attachConfig(ctx, loader)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamInterceptor mirrors UnaryInterceptor for streaming RPCs.
func StreamInterceptor(loader *Loader) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, err := attachConfig(ss.Context(), loader)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// attachConfig loads the vertical config for the tenant in context.
// If no tenant ID is present (e.g. health checks), it passes through.
func attachConfig(ctx context.Context, loader *Loader) (context.Context, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		// No tenant in context — unauthenticated call (health check, reflection).
		return ctx, nil
	}

	cfg, err := loader.GetConfig(ctx, tenantID)
	if err != nil {
		return ctx, status.Errorf(codes.Internal,
			"vertical: failed to load config for tenant %s: %v", tenantID, err)
	}

	return WithConfig(ctx, cfg), nil
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
