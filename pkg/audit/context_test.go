package audit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithActor_RoundTrip(t *testing.T) {
	t.Parallel()
	actor := ActorInfo{
		UserID: uuid.MustParse("d1000000-0000-0000-0000-000000000001"),
		Email:  "admin@xyzcba.com",
		IP:     "10.0.0.1",
	}

	ctx := WithActor(context.Background(), actor)
	got, ok := ActorFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, actor.UserID, got.UserID)
	assert.Equal(t, actor.Email, got.Email)
	assert.Equal(t, actor.IP, got.IP)
}

func TestActorFromContext_Empty(t *testing.T) {
	t.Parallel()
	_, ok := ActorFromContext(context.Background())
	assert.False(t, ok)
}
