package vertical

import "context"

type contextKey struct{}

// WithConfig stores the vertical Config in the context.
func WithConfig(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext retrieves the vertical Config from the context.
// Returns nil, false if no config is present.
func FromContext(ctx context.Context) (*Config, bool) {
	cfg, ok := ctx.Value(contextKey{}).(*Config)
	return cfg, ok
}

// MustFromContext retrieves the Config and panics if not present.
// Use only in handlers guaranteed to run behind the vertical interceptor.
func MustFromContext(ctx context.Context) *Config {
	cfg, ok := FromContext(ctx)
	if !ok {
		panic("vertical: MustFromContext called without vertical config in context — ensure the vertical interceptor is registered in the gRPC chain")
	}
	return cfg
}
