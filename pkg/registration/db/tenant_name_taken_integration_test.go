//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/registration"
	regdb "github.com/wegofwd2020/thittam/pkg/registration/db"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestStore_CreateTenant_NameTaken(t *testing.T) {
	pool := testdb.Open(t)
	store := regdb.NewStore(pool, nil) // vq nil: tenant methods don't use it

	const name = "Reg Parity  Studios" // double internal space
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM tenants WHERE regexp_replace(lower(trim(name)),'\s+',' ','g')
			 = regexp_replace(lower(trim($1)),'\s+',' ','g')`, name)
	})

	_, err := store.CreateTenant(context.Background(), name, "reg-parity-studios", "starter")
	require.NoError(t, err)

	// Case + whitespace varied duplicate → clean sentinel, not a raw pg error.
	_, err = store.CreateTenant(context.Background(), "reg parity studios", "reg-parity-studios-2", "starter")
	require.Error(t, err)
	assert.ErrorIs(t, err, registration.ErrTenantNameTaken)

	// Pre-flight existence check agrees.
	exists, err := store.TenantExistsByNormalizedName(context.Background(), "  REG PARITY studios ")
	require.NoError(t, err)
	assert.True(t, exists)

	absent, err := store.TenantExistsByNormalizedName(context.Background(), "Nobody Reg Inc")
	require.NoError(t, err)
	assert.False(t, absent)
}
