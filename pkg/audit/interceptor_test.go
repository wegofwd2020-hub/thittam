package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryInterceptor_WithActor(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    10,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	interceptor := UnaryInterceptor(logger)

	// Build context with actor + tenant
	ctx := WithActor(context.Background(), ActorInfo{
		UserID: testActorID,
		Email:  "admin@xyzcba.com",
		IP:     "10.0.0.1",
	})
	ctx = tenant.WithID(ctx, testTenantID)

	info := &grpc.UnaryServerInfo{FullMethod: "/thittam.expense.v1.ExpenseService/ApproveExpense"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	// Wait for flush
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, logger.Close(context.Background()))

	events := store.getEvents()
	require.Len(t, events, 1)
	assert.Equal(t, testActorID, events[0].ActorID)
	assert.Equal(t, testTenantID, events[0].TenantID)
	assert.Equal(t, "admin@xyzcba.com", events[0].ActorEmail)
}

func TestUnaryInterceptor_NoActor_SkipsAudit(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    10,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	interceptor := UnaryInterceptor(logger)

	// No actor in context (unauthenticated call)
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	time.Sleep(150 * time.Millisecond)
	require.NoError(t, logger.Close(context.Background()))

	assert.Equal(t, 0, store.count())
}

func TestUnaryInterceptor_HandlerError_RecordsGRPCCode(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    10,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	interceptor := UnaryInterceptor(logger)

	ctx := WithActor(context.Background(), ActorInfo{
		UserID: testActorID,
		Email:  "admin@xyzcba.com",
	})
	ctx = tenant.WithID(ctx, testTenantID)

	info := &grpc.UnaryServerInfo{FullMethod: "/thittam.expense.v1.ExpenseService/ApproveExpense"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.PermissionDenied, "approval limit exceeded")
	}

	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	time.Sleep(150 * time.Millisecond)
	require.NoError(t, logger.Close(context.Background()))

	events := store.getEvents()
	require.Len(t, events, 1)

	// Verify metadata contains the gRPC code
	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(events[0].Metadata, &meta))
	assert.Equal(t, "PermissionDenied", meta["grpc_code"])
}

func TestStreamInterceptor_WithActor(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    10,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	interceptor := StreamInterceptor(logger)

	ctx := WithActor(context.Background(), ActorInfo{
		UserID: testActorID,
		Email:  "admin@xyzcba.com",
	})
	ctx = tenant.WithID(ctx, testTenantID)

	ss := &mockServerStream{ctx: ctx}
	info := &grpc.StreamServerInfo{FullMethod: "/thittam.reporting.v1.ReportingService/RunReport"}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)
	require.NoError(t, logger.Close(context.Background()))

	assert.Equal(t, 1, store.count())
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

func TestBuildEventFromContext_ExtractsTenant(t *testing.T) {
	tenantID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	actorID := uuid.MustParse("b0000000-0000-0000-0000-000000000001")

	ctx := WithActor(context.Background(), ActorInfo{
		UserID: actorID,
		Email:  "user@example.com",
		IP:     "192.168.1.1",
	})
	ctx = tenant.WithID(ctx, tenantID)

	event := buildEventFromContext(ctx, "/test/Method", nil, 42*time.Millisecond)
	require.NotNil(t, event)
	assert.Equal(t, tenantID, event.TenantID)
	assert.Equal(t, actorID, event.ActorID)
	assert.Equal(t, "192.168.1.1", event.ActorIP)
}
