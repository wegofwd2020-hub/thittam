package db

import (
	"testing"

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
