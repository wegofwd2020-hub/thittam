//go:build integration

// Integration test for the case-insensitive UNIQUE index on tenants.name
// added in migration 015 (#91). Verifies:
//   - Second insert with case-varied name collides
//   - Second insert with leading/trailing whitespace collides (index uses trim)
//   - Collision surfaces as pgconn.PgError code 23505 with ConstraintName =
//     "tenants_name_ci_unique" so the repo-layer isUniqueViolationOn helper
//     can map it to iam.ErrTenantNameTaken
//
// Each test wraps work in testdb.NewTx so rollback is automatic.

package db_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func insertTenant(t *testing.T, tx pgx.Tx, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, name, "slug-"+id.String()[:8])
	require.NoError(t, err, "insert tenant %q", name)
	return id
}

func assertUniqueViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "expected pgconn.PgError, got %T", err)
	assert.Equal(t, "23505", pgErr.Code, "expected unique_violation, got %s", pgErr.Code)
	assert.Equal(t, constraint, pgErr.ConstraintName, "expected violation on %s", constraint)
}

func TestTenantsNameUnique_CaseInsensitive(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	insertTenant(t, tx, "Acme Studios")

	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, "acme studios", "slug-"+id.String()[:8])
	assertUniqueViolation(t, err, "tenants_name_ci_unique")
}

func TestTenantsNameUnique_AllCaps(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	insertTenant(t, tx, "Acme Studios")

	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, "ACME STUDIOS", "slug-"+id.String()[:8])
	assertUniqueViolation(t, err, "tenants_name_ci_unique")
}

func TestTenantsNameUnique_LeadingTrailingWhitespaceTrimmed(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	insertTenant(t, tx, "Acme Studios")

	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, "  Acme Studios  ", "slug-"+id.String()[:8])
	assertUniqueViolation(t, err, "tenants_name_ci_unique")
}

func TestTenantsNameUnique_DistinctNamesCoexist(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	insertTenant(t, tx, "Acme Studios")
	insertTenant(t, tx, "Beta Studios")
}

func TestTenantsNameUnique_InternalWhitespaceCollapsed(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	insertTenant(t, tx, "Acme  Corp") // two spaces

	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, "Acme Corp", "slug-"+id.String()[:8]) // one space
	assertUniqueViolation(t, err, "tenants_name_ci_unique")
}

func TestCreateTenant_ConcurrentSameName_Race(t *testing.T) {
	pool := testdb.Open(t)
	// NB: not NewTx — concurrent goroutines need independent connections,
	// so use the pool directly and clean up the inserted row(s) at the end.
	const name = "Race Condition Studios"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM tenants WHERE regexp_replace(lower(trim(name)),'\s+',' ','g')
			 = regexp_replace(lower(trim($1)),'\s+',' ','g')`, name)
	})

	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := uuid.New()
			_, err := pool.Exec(context.Background(),
				`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
				 VALUES ($1, $2, $3, 'US', 'USD')`,
				id, name, "slug-"+id.String()[:8])
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	var ok, dup int
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		var pgErr *pgconn.PgError
		if assert.ErrorAs(t, err, &pgErr) && pgErr.Code == "23505" {
			dup++
		}
	}
	assert.Equal(t, 1, ok, "exactly one insert should win")
	assert.Equal(t, n-1, dup, "the rest should be 23505 duplicates")
}
