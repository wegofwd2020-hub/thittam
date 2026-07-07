package db

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/registration"
)

func TestStore_ImplementsTenantStore(t *testing.T) {
	var _ registration.TenantStore = (*Store)(nil)
}

func TestStore_ImplementsVerticalStore(t *testing.T) {
	var _ registration.VerticalStore = (*Store)(nil)
}

func TestNewStore(t *testing.T) {
	// NewStore requires a *sql.DB, which we can't easily create without a real DB.
	// This test validates the constructor signature compiles.
	assert.NotNil(t, NewStore)
}

func TestNewQueries(t *testing.T) {
	// Verify Queries constructor works with a nil DBTX (won't panic until called).
	q := New(nil)
	require.NotNil(t, q)
}

func TestIsUniqueViolationOn(t *testing.T) {
	t.Parallel()
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "tenants_name_ci_unique"}
	assert.True(t, isUniqueViolationOn(pgErr, "tenants_name_ci_unique"))
	assert.False(t, isUniqueViolationOn(pgErr, "other_index"))
	assert.False(t, isUniqueViolationOn(&pgconn.PgError{Code: "23503", ConstraintName: "tenants_name_ci_unique"}, "tenants_name_ci_unique"))
	assert.False(t, isUniqueViolationOn(errors.New("plain"), "tenants_name_ci_unique"))
}
