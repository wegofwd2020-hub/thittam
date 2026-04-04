package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryInterceptor returns a gRPC unary interceptor that logs audit events
// for every RPC call. It extracts actor and tenant info from the context
// (set by auth and tenant interceptors) and records the call outcome.
//
// Place this interceptor AFTER auth and vertical interceptors in the chain.
func UnaryInterceptor(logger *Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// Build audit event from context
		event := buildEventFromContext(ctx, info.FullMethod, err, duration)
		if event != nil {
			logger.Log(*event)
		}

		return resp, err
	}
}

// StreamInterceptor returns a gRPC stream interceptor for audit logging.
func StreamInterceptor(logger *Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start)

		event := buildEventFromContext(ss.Context(), info.FullMethod, err, duration)
		if event != nil {
			logger.Log(*event)
		}

		return err
	}
}

// buildEventFromContext constructs an audit Event from the request context.
// Returns nil if actor info is not present (unauthenticated calls).
func buildEventFromContext(ctx context.Context, method string, err error, duration time.Duration) *Event {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return nil // unauthenticated call — skip audit
	}

	tenantID, _ := tenant.IDFromContext(ctx)

	grpcCode := "OK"
	if err != nil {
		if st, ok := status.FromError(err); ok {
			grpcCode = st.Code().String()
		} else {
			grpcCode = "UNKNOWN"
		}
	}

	return &Event{
		ID:           uuid.New(),
		TenantID:     tenantID,
		ActorID:      actor.UserID,
		ActorEmail:   actor.Email,
		ActorIP:      actor.IP,
		Action:       Action("rpc_call"),
		ResourceType: ResourceType("grpc_method"),
		ResourceID:   uuid.Nil, // RPC calls don't have a single resource ID
		Metadata:     marshalMetadata(map[string]interface{}{
			"method":    method,
			"grpc_code": grpcCode,
			"duration_ms": duration.Milliseconds(),
		}),
		OccurredAt: time.Now(),
	}
}

func marshalMetadata(m map[string]interface{}) []byte {
	data, _ := json.Marshal(m)
	return data
}
