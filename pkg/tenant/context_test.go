package tenant

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestWithID_IDFromContext(t *testing.T) {
	id := uuid.New()
	ctx := WithID(context.Background(), id)

	got, ok := IDFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, id, got)
}

func TestIDFromContext_EmptyContext(t *testing.T) {
	got, ok := IDFromContext(context.Background())
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}
