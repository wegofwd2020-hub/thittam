package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ForwardAuthUnaryClientInterceptor copies the caller's `authorization` metadata
// from the incoming server context onto the outgoing client call, so a downstream
// service verifies the same token and acts with exactly the caller's authority.
//
// gRPC does not propagate incoming metadata to outgoing calls. Without this, a
// service-to-service call arrives at a fail-closed callee with no credentials
// (#138).
//
// It forwards ONLY `authorization` (plus `x-forwarded-for`, to keep the audit
// trail pointing at the original client IP). It must never forward x-caller-*
// or x-tenant-id: those confer nothing, and copying them would resurrect the
// header-trust this change removed.
func ForwardAuthUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		in, _ := metadata.FromIncomingContext(ctx)
		out, _ := metadata.FromOutgoingContext(ctx)

		if authz := firstMD(in, "authorization"); authz != "" && len(out.Get("authorization")) == 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authz)
		}

		if xff := firstMD(in, "x-forwarded-for"); xff != "" && len(out.Get("x-forwarded-for")) == 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, "x-forwarded-for", xff)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
