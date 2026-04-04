package audit

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct{}

// ActorInfo holds the audit actor metadata extracted from the request context.
type ActorInfo struct {
	UserID uuid.UUID
	Email  string
	IP     string
}

// WithActor attaches actor information to the context for audit logging.
// Typically called by the auth interceptor after JWT validation.
func WithActor(ctx context.Context, actor ActorInfo) context.Context {
	return context.WithValue(ctx, contextKey{}, actor)
}

// ActorFromContext retrieves the actor info from context, if present.
func ActorFromContext(ctx context.Context) (ActorInfo, bool) {
	actor, ok := ctx.Value(contextKey{}).(ActorInfo)
	return actor, ok
}
