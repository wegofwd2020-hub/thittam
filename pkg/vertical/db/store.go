package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Store implements the vertical.DB interface using sqlc-generated queries.
// It bridges the Loader's data access needs with the PostgreSQL backend.
type Store struct {
	q *Queries
}

// NewStore creates a Store backed by the given database connection.
func NewStore(db DBTX) *Store {
	return &Store{q: New(db)}
}

// GetVerticalConfigForTenant returns the merged vertical config JSON for a tenant.
// Returns vertical.ErrNotFound if the tenant has no bound vertical.
func (s *Store) GetVerticalConfigForTenant(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	data, err := s.q.GetVerticalConfigForTenant(ctx, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant %s: %w", tenantID, vertical.ErrNotFound)
		}
		return nil, fmt.Errorf("query vertical config for tenant %s: %w", tenantID, err)
	}
	return data, nil
}

// GetVerticalConfigWithDeepMerge returns the vertical config with deep-merge applied.
// Uses the structured override format (append/override/remove) instead of PostgreSQL
// shallow merge. Returns vertical.ErrNotFound if the tenant has no bound vertical.
func (s *Store) GetVerticalConfigWithDeepMerge(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	result, err := s.q.GetVerticalConfigAndOverride(ctx, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
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
