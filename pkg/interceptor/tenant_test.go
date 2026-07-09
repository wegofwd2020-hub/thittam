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

// The returned tenant must never be the request's, even when the request names a
// tenant that happens to parse. This is the whole point of the helper.
func TestTenantFromRequest_NeverReturnsTheRequestTenant(t *testing.T) {
	t.Parallel()
	tenantA, tenantB := uuid.New(), uuid.New()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: tenantA})

	got, err := TenantFromRequest(ctx, tenantB.String())
	require.Error(t, err)
	assert.NotEqual(t, tenantB, got)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
