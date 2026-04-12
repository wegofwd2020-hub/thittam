package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// CacheInvalidator is satisfied by *vertical.Loader.
// Injected into Store so that write methods can bust the Redis cache immediately
// after the DB write succeeds, preventing stale vertical configs from being served
// for up to the 5-minute TTL window.
type CacheInvalidator interface {
	Invalidate(ctx context.Context, tenantID uuid.UUID) error
}

// Store implements the vertical.DB interface using sqlc-generated queries.
// It bridges the Loader's data access needs with the PostgreSQL backend.
type Store struct {
	q   *Queries
	inv CacheInvalidator // optional; nil means no cache invalidation
}

// NewStore creates a Store backed by the given database connection.
func NewStore(db DBTX) *Store {
	return &Store{q: New(db)}
}

// WithCacheInvalidator attaches a CacheInvalidator to the store. Every write
// method (BindTenant, UpdateOverride, UnbindTenant) calls Invalidate after a
// successful DB write. Returns the receiver for chaining.
func (s *Store) WithCacheInvalidator(inv CacheInvalidator) *Store {
	s.inv = inv
	return s
}

// GetVerticalConfigForTenant returns the merged vertical config JSON for a tenant.
// Returns vertical.ErrNotFound if the tenant has no bound vertical.
func (s *Store) GetVerticalConfigForTenant(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	data, err := s.q.GetVerticalConfigForTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("tenant %s: %w", tenantID, vertical.ErrNotFound)
		}
		return nil, fmt.Errorf("query vertical config for tenant %s: %w", tenantID, err)
	}
	// pgx scans untyped jsonb expressions into []byte when scanning to interface{}.
	if b, ok := data.([]byte); ok {
		return b, nil
	}
	return json.Marshal(data)
}

// GetVerticalConfigWithDeepMerge returns the vertical config with deep-merge applied.
// Uses the structured override format (append/override/remove) instead of PostgreSQL
// shallow merge. Returns vertical.ErrNotFound if the tenant has no bound vertical.
func (s *Store) GetVerticalConfigWithDeepMerge(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	result, err := s.q.GetVerticalConfigAndOverride(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("tenant %s: %w", tenantID, vertical.ErrNotFound)
		}
		return nil, fmt.Errorf("query vertical config for tenant %s: %w", tenantID, err)
	}

	// Parse base config
	var base vertical.Config
	if err := json.Unmarshal(result.BaseConfig, &base); err != nil {
		return nil, fmt.Errorf("unmarshal base config: %w", err)
	}

	// Apply deep-merge if override exists
	merged, err := vertical.ApplyOverride(&base, result.ConfigOverride)
	if err != nil {
		return nil, fmt.Errorf("apply override for tenant %s: %w", tenantID, err)
	}

	// Re-serialize the merged config
	data, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config: %w", err)
	}

	return data, nil
}

// Queries returns the underlying sqlc Queries for direct access to all queries.
func (s *Store) Queries() *Queries {
	return s.q
}

// BindTenant binds a tenant to a vertical. On conflict (tenant already bound),
// this is a no-op. Calls Invalidate on the cache after a successful write.
func (s *Store) BindTenant(ctx context.Context, params BindTenantVerticalParams) error {
	if err := s.q.BindTenantVertical(ctx, params); err != nil {
		return fmt.Errorf("bind tenant vertical: %w", err)
	}
	s.invalidate(ctx, params.TenantID)
	return nil
}

// UpdateOverride replaces the tenant-specific vertical config override.
// Calls Invalidate on the cache after a successful write so the updated config
// is served on the next request rather than after the TTL expires.
func (s *Store) UpdateOverride(ctx context.Context, params UpdateTenantVerticalOverrideParams) error {
	if err := s.q.UpdateTenantVerticalOverride(ctx, params); err != nil {
		return fmt.Errorf("update tenant vertical override: %w", err)
	}
	s.invalidate(ctx, params.TenantID)
	return nil
}

// UnbindTenant removes a tenant's vertical binding. Calls Invalidate so the
// now-missing config is not served from cache.
func (s *Store) UnbindTenant(ctx context.Context, tenantID uuid.UUID) error {
	if err := s.q.UnbindTenantVertical(ctx, tenantID); err != nil {
		return fmt.Errorf("unbind tenant vertical: %w", err)
	}
	s.invalidate(ctx, tenantID)
	return nil
}

// invalidate busts the Redis cache entry for the tenant if an invalidator is
// configured. Errors are swallowed here — a stale cache entry is preferable to
// propagating a Redis error through a business operation that already succeeded.
func (s *Store) invalidate(ctx context.Context, tenantID uuid.UUID) {
	if s.inv == nil {
		return
	}
	_ = s.inv.Invalidate(ctx, tenantID)
}
