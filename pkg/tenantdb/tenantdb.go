// Package tenantdb provides safe tenant-schema routing for PostgreSQL connections.
//
// Thittam uses a tenant-per-schema isolation model: every tenant's data lives in
// a dedicated schema named tenant_<uuid>. Queries are routed to the correct schema
// by setting the PostgreSQL search_path on the connection before executing them.
//
// Security note — injection risk in SET search_path
// =================================================
// SET search_path does not support query parameters ($1 placeholders). The schema
// name must be interpolated into the SQL string. This creates a schema injection
// risk if the tenant ID is ever sourced from user-controlled input without
// validation.
//
// This package eliminates that risk by:
//  1. Accepting only uuid.UUID values (already a validated type in Go) as the
//     tenant identifier. Callers cannot pass an arbitrary string.
//  2. Explicitly rejecting uuid.Nil (the zero UUID), which has no corresponding
//     tenant schema.
//  3. Using uuid.UUID.String() for interpolation — this method always returns a
//     well-formed 36-character UUID string containing only hex digits and hyphens.
//     It cannot contain SQL metacharacters.
//
// Usage
// =====
//
//	conn, release, err := tenantdb.Acquire(ctx, pool, tenantID)
//	if err != nil {
//	    return err
//	}
//	defer release()
//	// conn is scoped to tenant_<tenantID> for the duration of this call.
//	rows, err := conn.QueryContext(ctx, "SELECT id, name FROM productions")
package tenantdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrInvalidTenantID is returned when the tenant ID fails validation before
// the search_path SET command. Any caller that receives this error has
// supplied a zero UUID — it is always a programming error, not a recoverable
// runtime condition.
var ErrInvalidTenantID = errors.New("tenantdb: tenant ID must be a non-nil UUID")

// Conn is a single database connection scoped to a tenant schema.
// Call Release when done — this returns the underlying connection to the pool.
type Conn struct {
	*sql.Conn
}

// Acquire checks out a connection from pool, validates tenantID, and sets
// search_path = tenant_<tenantID>, public on the connection.
//
// The returned release function MUST be called when the connection is no
// longer needed (typically via defer). Failing to call release leaks the
// connection from the pool.
//
//	conn, release, err := tenantdb.Acquire(ctx, pool, tenantID)
//	if err != nil { ... }
//	defer release()
func Acquire(ctx context.Context, pool *sql.DB, tenantID uuid.UUID) (*Conn, func(), error) {
	// Validate before any interpolation. uuid.Nil has no corresponding schema.
	if tenantID == uuid.Nil {
		return nil, noop, ErrInvalidTenantID
	}

	// uuid.UUID.String() always returns "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" —
	// 36 characters of lowercase hex and hyphens. No SQL metacharacters are
	// possible in this representation.
	schemaName := schemaNameFor(tenantID)

	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, noop, fmt.Errorf("tenantdb: acquire connection: %w", err)
	}

	// SET search_path is a session-level command on this specific connection.
	// We include "public" so shared functions and extensions remain accessible.
	//
	// Note: SET search_path cannot use query parameters ($1) in PostgreSQL —
	// string interpolation here is intentional and safe because schemaName
	// is derived exclusively from uuid.UUID.String().
	if _, err := conn.ExecContext(ctx, "SET search_path = "+schemaName+", public"); err != nil {
		_ = conn.Close()
		return nil, noop, fmt.Errorf("tenantdb: set search_path for tenant %s: %w", tenantID, err)
	}

	release := func() { _ = conn.Close() }
	return &Conn{conn}, release, nil
}

// schemaNameFor returns the canonical PostgreSQL schema name for a tenant.
// Format: tenant_<uuid> where <uuid> is the standard hyphenated lowercase form.
// This is the single source of truth for the schema naming convention.
func schemaNameFor(id uuid.UUID) string {
	return "tenant_" + id.String()
}

func noop() {}
