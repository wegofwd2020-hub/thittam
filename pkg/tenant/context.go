package tenant

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct{}

// WithID stores the tenant UUID in the context.
func WithID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// IDFromContext retrieves the tenant UUID from the context.
// Returns uuid.Nil, false if not present.
func IDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(contextKey{}).(uuid.UUID)
	return id, ok
}
