package vertical

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithConfig_FromContext(t *testing.T) {
	cfg := &Config{ID: "test-vertical", Name: "Test"}
	ctx := WithConfig(context.Background(), cfg)

	got, ok := FromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "test-vertical", got.ID)
}

func TestFromContext_EmptyContext(t *testing.T) {
	got, ok := FromContext(context.Background())
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestMustFromContext_Success(t *testing.T) {
	cfg := &Config{ID: "test-vertical"}
	ctx := WithConfig(context.Background(), cfg)

	got := MustFromContext(ctx)
	assert.Equal(t, "test-vertical", got.ID)
}

func TestMustFromContext_Panics(t *testing.T) {
	assert.PanicsWithValue(t,
		"vertical: MustFromContext called without vertical config in context — ensure the vertical interceptor is registered in the gRPC chain",
		func() { MustFromContext(context.Background()) },
	)
}
