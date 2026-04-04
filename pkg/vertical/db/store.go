package db

import (
	"context"
	"database/sql"
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

// Queries returns the underlying sqlc Queries for direct access to all queries.
func (s *Store) Queries() *Queries {
	return s.q
}
