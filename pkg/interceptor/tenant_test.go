package interceptor

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTenantFromRequest(t *testing.T) {
	t.Parallel()

	tenantA := uuid.New()
	tenantB := uuid.New()

	callerIn := func(tid uuid.UUID) context.Context {
		return WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: tid})
	}

	tests := []struct {
		name    string
		ctx     context.Context
		reqID   string
		want    uuid.UUID
		wantErr codes.Code
	}{
		{"no caller in context", context.Background(), "", uuid.Nil, codes.Unauthenticated},
		{"caller with nil tenant", callerIn(uuid.Nil), "", uuid.Nil, codes.Unauthenticated},
		{"empty request tenant uses the token", callerIn(tenantA), "", tenantA, codes.OK},
		{"matching request tenant", callerIn(tenantA), tenantA.String(), tenantA, codes.OK},
		{"unparseable request tenant", callerIn(tenantA), "not-a-uuid", uuid.Nil, codes.InvalidArgument},
		{"MISMATCHED request tenant is refused", callerIn(tenantA), tenantB.String(), uuid.Nil, codes.PermissionDenied},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := TenantFromRequest(tc.ctx, tc.reqID)
			if tc.wantErr != codes.OK {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr, status.Code(err))
				assert.Equal(t, uuid.Nil, got, "no tenant may be returned alongside an error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// On a mismatch the helper must return neither tenant — not the request's (the
// obvious leak) and not the caller's (which a handler ignoring the error would
// then use to query, silently succeeding on the wrong tenant).
func TestTenantFromRequest_NeverReturnsTheRequestTenant(t *testing.T) {
	t.Parallel()
	tenantA, tenantB := uuid.New(), uuid.New()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: tenantA})

	got, err := TenantFromRequest(ctx, tenantB.String())
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, uuid.Nil, got)
	assert.NotEqual(t, tenantB, got, "the request's tenant must never be returned")
	assert.NotEqual(t, tenantA, got, "nor the caller's, alongside an error")
}

// The literal nil UUID parses. It must not become a wildcard: the nil-caller-tenant
// guard runs before parsing, so a real caller sending it gets PermissionDenied.
func TestTenantFromRequest_NilUUIDRequestIsNotAWildcard(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: uuid.New()})

	got, err := TenantFromRequest(ctx, uuid.Nil.String())
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, uuid.Nil, got)
}

// Whitespace is a malformed id, not an omitted one. It must not fall into the
// "empty means use the token's tenant" branch.
func TestTenantFromRequest_WhitespaceIsNotEmpty(t *testing.T) {
	t.Parallel()
	tenantA := uuid.New()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: tenantA})

	for _, in := range []string{" ", "\t", "\n"} {
		got, err := TenantFromRequest(ctx, in)
		require.Error(t, err, "input %q", in)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Equal(t, uuid.Nil, got)
	}
}
