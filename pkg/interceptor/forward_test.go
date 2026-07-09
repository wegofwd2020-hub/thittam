package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// captureInvoker returns a grpc.UnaryInvoker that records the outgoing
// metadata seen on ctx instead of making a real call.
func captureInvoker(t *testing.T) (grpc.UnaryInvoker, *metadata.MD) {
	t.Helper()
	var captured metadata.MD
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		captured = md
		return nil
	}
	return invoker, &captured
}

func TestForwardAuthUnaryClientInterceptor_ForwardsAuthorization(t *testing.T) {
	incoming := metadata.Pairs("authorization", "Bearer abc")
	ctx := metadata.NewIncomingContext(context.Background(), incoming)

	invoker, captured := captureInvoker(t)
	err := ForwardAuthUnaryClientInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)

	assert.Equal(t, []string{"Bearer abc"}, captured.Get("authorization"))
}

func TestForwardAuthUnaryClientInterceptor_NoAuthorization(t *testing.T) {
	incoming := metadata.Pairs("x-forwarded-for", "10.0.0.1")
	ctx := metadata.NewIncomingContext(context.Background(), incoming)

	invoker, captured := captureInvoker(t)
	err := ForwardAuthUnaryClientInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)

	assert.Empty(t, captured.Get("authorization"))
}

func TestForwardAuthUnaryClientInterceptor_DoesNotForwardCallerHeaders(t *testing.T) {
	incoming := metadata.Pairs(
		"authorization", "Bearer abc",
		"x-caller-role", "platform_admin",
		"x-caller-id", "22222222-2222-2222-2222-222222222222",
		"x-caller-email", "attacker@evil.example",
		"x-tenant-id", "11111111-1111-1111-1111-111111111111",
		"x-project-id", "33333333-3333-3333-3333-333333333333",
	)
	ctx := metadata.NewIncomingContext(context.Background(), incoming)

	invoker, captured := captureInvoker(t)
	err := ForwardAuthUnaryClientInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)

	assert.Equal(t, []string{"Bearer abc"}, captured.Get("authorization"))

	// Nothing but authorization and x-forwarded-for may cross the boundary.
	// Forwarding any identity header would relocate the header-trust that #138
	// removed, rather than remove it.
	for k := range *captured {
		assert.Contains(t, []string{"authorization", "x-forwarded-for"}, k,
			"forwarded unexpected metadata key %q", k)
	}
	assert.Empty(t, captured.Get("x-caller-role"), "x-caller-role must never be forwarded downstream")
	assert.Empty(t, captured.Get("x-tenant-id"), "x-tenant-id must never be forwarded downstream")
}

func TestForwardAuthUnaryClientInterceptor_ForwardsXForwardedFor(t *testing.T) {
	incoming := metadata.Pairs("authorization", "Bearer abc", "x-forwarded-for", "203.0.113.7")
	ctx := metadata.NewIncomingContext(context.Background(), incoming)

	invoker, captured := captureInvoker(t)
	err := ForwardAuthUnaryClientInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)

	assert.Equal(t, []string{"203.0.113.7"}, captured.Get("x-forwarded-for"))
}

func TestForwardAuthUnaryClientInterceptor_DoesNotDuplicateExistingAuthorization(t *testing.T) {
	incoming := metadata.Pairs("authorization", "Bearer abc")
	ctx := metadata.NewIncomingContext(context.Background(), incoming)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer existing")

	invoker, captured := captureInvoker(t)
	err := ForwardAuthUnaryClientInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)

	assert.Equal(t, []string{"Bearer existing"}, captured.Get("authorization"))
}
