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

func TestActorFromRequest(t *testing.T) {
	t.Parallel()

	userA := uuid.New()
	userB := uuid.New()

	callerIn := func(uid uuid.UUID) context.Context {
		return WithCaller(context.Background(), CallerInfo{UserID: uid, TenantID: uuid.New()})
	}

	tests := []struct {
		name    string
		ctx     context.Context
		reqID   string
		want    uuid.UUID
		wantErr codes.Code
	}{
		{"no caller in context", context.Background(), "", uuid.Nil, codes.Unauthenticated},
		{"caller with nil subject", callerIn(uuid.Nil), "", uuid.Nil, codes.Unauthenticated},
		{"empty request actor uses the token", callerIn(userA), "", userA, codes.OK},
		{"matching request actor", callerIn(userA), userA.String(), userA, codes.OK},
		{"unparseable request actor", callerIn(userA), "not-a-uuid", uuid.Nil, codes.InvalidArgument},
		{"MISMATCHED request actor is refused", callerIn(userA), userB.String(), uuid.Nil, codes.PermissionDenied},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ActorFromRequest(tc.ctx, tc.reqID)
			if tc.wantErr == codes.OK {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, status.Code(err))
			assert.Equal(t, uuid.Nil, got, "every error path must return uuid.Nil")
		})
	}
}

// The mismatch error must not confirm the existence of the other user id.
func TestActorFromRequest_MismatchIsNotAnOracle(t *testing.T) {
	t.Parallel()
	userA, userB := uuid.New(), uuid.New()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: userA, TenantID: uuid.New()})

	_, err := ActorFromRequest(ctx, userB.String())
	require.Error(t, err)
	msg := status.Convert(err).Message()
	assert.NotContains(t, msg, userA.String())
	assert.NotContains(t, msg, userB.String())
}
