package vertical

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupInterceptorLoader(t *testing.T, db DB) *Loader {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return NewLoader(rdb, db, nil)
}

func TestUnaryInterceptor_WithTenant(t *testing.T) {
	tenantID := uuid.New()
	cfg := &Config{ID: "movie-production", Name: "Movie Production", Version: "1.0.0"}
	data, _ := json.Marshal(cfg)
	db := &mockDB{data: map[uuid.UUID][]byte{tenantID: data}}
	loader := setupInterceptorLoader(t, db)

	interceptor := UnaryInterceptor(loader)

	ctx := tenant.WithID(context.Background(), tenantID)
	var handlerCtx context.Context

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCtx = ctx
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	// Verify config was injected into handler's context
	got, ok := FromContext(handlerCtx)
	require.True(t, ok)
	assert.Equal(t, "movie-production", got.ID)
}

func TestUnaryInterceptor_NoTenant_Passthrough(t *testing.T) {
	db := &mockDB{}
	loader := setupInterceptorLoader(t, db)

	interceptor := UnaryInterceptor(loader)

	called := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// No vertical config should be in context
		_, ok := FromContext(ctx)
		assert.False(t, ok)
		called = true
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, called)
}

func TestUnaryInterceptor_LoaderError(t *testing.T) {
	tenantID := uuid.New()
	db := &mockDB{err: errors.New("db down")}
	loader := setupInterceptorLoader(t, db)

	interceptor := UnaryInterceptor(loader)

	ctx := tenant.WithID(context.Background(), tenantID)
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

func TestStreamInterceptor_WithTenant(t *testing.T) {
	tenantID := uuid.New()
	cfg := &Config{ID: "software-dev", Version: "1.0.0"}
	data, _ := json.Marshal(cfg)
	db := &mockDB{data: map[uuid.UUID][]byte{tenantID: data}}
	loader := setupInterceptorLoader(t, db)

	interceptor := StreamInterceptor(loader)

	ctx := tenant.WithID(context.Background(), tenantID)
	stream := &mockServerStream{ctx: ctx}

	var streamCtx context.Context
	handler := func(srv interface{}, ss grpc.ServerStream) error {
		streamCtx = ss.Context()
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	require.NoError(t, err)

	got, ok := FromContext(streamCtx)
	require.True(t, ok)
	assert.Equal(t, "software-dev", got.ID)
}

func TestStreamInterceptor_NoTenant_Passthrough(t *testing.T) {
	db := &mockDB{}
	loader := setupInterceptorLoader(t, db)

	interceptor := StreamInterceptor(loader)
	stream := &mockServerStream{ctx: context.Background()}

	called := false
	handler := func(srv interface{}, ss grpc.ServerStream) error {
		_, ok := FromContext(ss.Context())
		assert.False(t, ok)
		called = true
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestStreamInterceptor_LoaderError(t *testing.T) {
	tenantID := uuid.New()
	db := &mockDB{err: errors.New("db down")}
	loader := setupInterceptorLoader(t, db)

	interceptor := StreamInterceptor(loader)

	ctx := tenant.WithID(context.Background(), tenantID)
	stream := &mockServerStream{ctx: ctx}

	handler := func(srv interface{}, ss grpc.ServerStream) error {
		t.Fatal("handler should not be called")
		return nil
	}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
