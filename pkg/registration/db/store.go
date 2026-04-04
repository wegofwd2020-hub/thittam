package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/registration"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
)

// Store implements registration.TenantStore and registration.VerticalStore.
type Store struct {
	q  *Queries
	vq *verticaldb.Queries
	db *sql.DB
}

// NewStore creates a Store backed by the given database connection.
// The vq parameter provides access to vertical_definitions queries.
func NewStore(db *sql.DB, vq *verticaldb.Queries) *Store {
	return &Store{
		q:  New(db),
		vq: vq,
		db: db,
	}
}

// --- TenantStore implementation ---

func (s *Store) CreateTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
	t, err := s.q.CreateTenant(ctx, name, slug, plan)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create tenant: %w", err)
	}
	return t.ID, nil
}

func (s *Store) CreateUser(ctx context.Context, tenantID uuid.UUID, email, displayName, passwordHash string) (uuid.UUID, error) {
	u, err := s.q.CreateUser(ctx, tenantID, email, displayName, passwordHash)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	return u.ID, nil
}

func (s *Store) TenantExistsBySlug(ctx context.Context, slug string) (bool, error) {
	return s.q.TenantExistsBySlug(ctx, slug)
}

func (s *Store) UserExistsByEmail(ctx context.Context, email string) (bool, error) {
	return s.q.UserExistsByEmail(ctx, email)
}

// --- VerticalStore implementation ---

func (s *Store) GetVerticalDefinition(ctx context.Context, id string) (registration.VerticalDefinitionRow, error) {
	vd, err := s.vq.GetVerticalDefinition(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return registration.VerticalDefinitionRow{}, registration.ErrVerticalNotFound
		}
		return registration.VerticalDefinitionRow{}, fmt.Errorf("get vertical %s: %w", id, err)
	}

	var desc string
	if vd.Description != nil {
		desc = *vd.Description
	}

	return registration.VerticalDefinitionRow{
		ID:          vd.ID,
		Name:        vd.Name,
		Version:     vd.Version,
		Description: desc,
		Config:      json.RawMessage(vd.Config),
		IsActive:    vd.IsActive,
	}, nil
}

func (s *Store) BindTenantVertical(ctx context.Context, tenantID uuid.UUID, verticalID string, registeredBy uuid.UUID) error {
	return s.vq.BindTenantVertical(ctx, verticaldb.BindTenantVerticalParams{
		TenantID:       tenantID,
		VerticalID:     verticalID,
		ConfigOverride: nil,
		RegisteredBy:   registeredBy,
	})
}

// --- Transaction support ---

// TxStore wraps Queries for use within a transaction.
type TxStore struct {
	q  *Queries
	vq *verticaldb.Queries
}

// InTx executes fn within a database transaction. The transaction is committed
// if fn returns nil, rolled back otherwise.
func (s *Store) InTx(ctx context.Context, fn func(tx TxStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	txStore := TxStore{
		q:  New(tx),
		vq: verticaldb.New(tx),
	}

	if err := fn(txStore); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// TxStore methods — mirror the Store methods but operate within the transaction.

func (ts TxStore) CreateTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
	t, err := ts.q.CreateTenant(ctx, name, slug, plan)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create tenant: %w", err)
	}
	return t.ID, nil
}

func (ts TxStore) CreateUser(ctx context.Context, tenantID uuid.UUID, email, displayName, passwordHash string) (uuid.UUID, error) {
	u, err := ts.q.CreateUser(ctx, tenantID, email, displayName, passwordHash)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	return u.ID, nil
}

func (ts TxStore) BindTenantVertical(ctx context.Context, tenantID uuid.UUID, verticalID string, registeredBy uuid.UUID) error {
	return ts.vq.BindTenantVertical(ctx, verticaldb.BindTenantVerticalParams{
		TenantID:       tenantID,
		VerticalID:     verticalID,
		ConfigOverride: nil,
		RegisteredBy:   registeredBy,
	})
}

// Compile-time interface checks.
var (
	_ registration.TenantStore   = (*Store)(nil)
	_ registration.VerticalStore = (*Store)(nil)
)
