package tenantdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- schemaNameFor ---

func TestSchemaNameFor(t *testing.T) {
	t.Parallel()

	t.Run("canonical format", func(t *testing.T) {
		t.Parallel()
		id := uuid.MustParse("d1000000-0000-0000-0000-000000000001")
		got := schemaNameFor(id)
		assert.Equal(t, "tenant_d1000000-0000-0000-0000-000000000001", got)
	})

	t.Run("no SQL metacharacters in output", func(t *testing.T) {
		t.Parallel()
		// The schema name is interpolated directly into a SET search_path
		// command. Assert that none of the characters that could be used for
		// SQL injection appear in the output. Dangerous chars: quotes,
		// semicolons, backslash, whitespace, parentheses, dollar sign.
		dangerous := "';\\\"() \t\n\r$"
		for i := 0; i < 100; i++ {
			name := schemaNameFor(uuid.New())
			for _, bad := range dangerous {
				assert.NotContainsf(t, name, string(bad),
					"dangerous character %q found in schema name %q", bad, name)
			}
		}
	})
}

// --- Acquire validation ---

func TestAcquire_RejectsNilUUID(t *testing.T) {
	t.Parallel()

	// nil pool — Acquire must fail on UUID validation before touching the pool.
	conn, release, err := Acquire(context.Background(), nil, uuid.Nil)

	assert.Nil(t, conn)
	assert.NotNil(t, release) // noop release is always returned
	assert.ErrorIs(t, err, ErrInvalidTenantID)

	// Calling release on the noop is safe.
	release()
}

func TestAcquire_RejectsZeroUUID(t *testing.T) {
	t.Parallel()

	var zero uuid.UUID // same as uuid.Nil — all bytes zero
	conn, release, err := Acquire(context.Background(), nil, zero)

	assert.Nil(t, conn)
	assert.ErrorIs(t, err, ErrInvalidTenantID)
	release()
}

// --- Acquire with a stub driver ---

// stubDriver records the queries executed on its connections so tests can
// assert the exact SET search_path command without a real database.
type stubConn struct {
	queries []string
	execErr error // if set, ExecContext returns this error
}

func (c *stubConn) Prepare(query string) (driver.Stmt, error) { return nil, nil }
func (c *stubConn) Close() error                              { return nil }
func (c *stubConn) Begin() (driver.Tx, error)                 { return nil, nil }
func (c *stubConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	c.queries = append(c.queries, query)
	if c.execErr != nil {
		return nil, c.execErr
	}
	return driver.RowsAffected(0), nil
}

type stubDriver struct {
	conn *stubConn
}

func (d *stubDriver) Open(_ string) (driver.Conn, error) { return d.conn, nil }

func newStubDB(t *testing.T, conn *stubConn) *sql.DB {
	t.Helper()
	// Register a uniquely named driver per test to avoid collisions.
	name := fmt.Sprintf("stub-%s", t.Name())
	sql.Register(name, &stubDriver{conn: conn})
	db, err := sql.Open(name, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAcquire_SetsSearchPath(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000001")
	stub := &stubConn{}
	db := newStubDB(t, stub)

	conn, release, err := Acquire(context.Background(), db, tenantID)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer release()

	require.Len(t, stub.queries, 1)
	assert.Equal(t,
		"SET search_path = tenant_d1000000-0000-0000-0000-000000000001, public",
		stub.queries[0],
	)
}

func TestAcquire_IncludesPublicInSearchPath(t *testing.T) {
	t.Parallel()

	// The public schema must be in the search_path so shared PostgreSQL
	// extensions and functions remain accessible.
	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000002")
	stub := &stubConn{}
	db := newStubDB(t, stub)

	_, release, err := Acquire(context.Background(), db, tenantID)
	require.NoError(t, err)
	defer release()

	require.Len(t, stub.queries, 1)
	assert.Contains(t, stub.queries[0], "public")
}

func TestAcquire_ReturnsErrorWhenSetSearchPathFails(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("d1000000-0000-0000-0000-000000000003")
	dbErr := errors.New("db: relation does not exist")
	stub := &stubConn{execErr: dbErr}
	db := newStubDB(t, stub)

	conn, release, err := Acquire(context.Background(), db, tenantID)

	assert.Nil(t, conn)
	assert.NotNil(t, release)
	assert.ErrorContains(t, err, "set search_path")
	// The tenant ID must appear in the error message for debuggability.
	assert.ErrorContains(t, err, tenantID.String())
	release()
}

func TestAcquire_SchemaNameNeverContainsMaliciousInput(t *testing.T) {
	t.Parallel()

	// Demonstrate that even if a caller somehow obtained a uuid.UUID value
	// with crafted bytes, uuid.String() always produces safe output.
	// UUID byte layout is fixed: 16 bytes → 32 hex chars + 4 hyphens.
	attacks := []string{
		"'; DROP SCHEMA tenant_foo; --",
		"../../../etc/passwd",
		"0\x00\x00\x00-0000-0000-0000-000000000000",
	}

	for _, attack := range attacks {
		// uuid.Parse rejects all of these as malformed UUIDs.
		_, err := uuid.Parse(attack)
		assert.Errorf(t, err,
			"uuid.Parse should reject malicious input %q", attack)
	}

	// Only well-formed UUIDs can reach schemaNameFor.
	// The output is always safe for interpolation.
	wellFormed := uuid.MustParse("d1000000-0000-0000-0000-000000000004")
	name := schemaNameFor(wellFormed)
	assert.Equal(t, "tenant_d1000000-0000-0000-0000-000000000004", name)
}
