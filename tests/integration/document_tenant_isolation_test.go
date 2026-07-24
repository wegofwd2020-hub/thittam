//go:build integration

// Integration test for the InitiateUpload folder cross-tenant fix (#174):
// InitiateUpload previously assigned req.FolderID to the new document without
// checking that the folder belongs to the caller's tenant, so a document's
// folder_id FK could point into another tenant's folder tree. MoveDocument
// already enforced this via repo.GetFolder(ctx, tenantID, folderID); this
// test proves InitiateUpload now does the same, against the actual Postgres
// repository (services/document/db.Postgres), not a double.
//
// Uses pkg/testdb (SKIPs without THITTAM_TEST_DSN); inserts rows directly via
// the pool and cleans them up in t.Cleanup, following the pattern in
// tests/integration/notifications_authz_test.go.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/document"
	documentdb "github.com/wegofwd2020/thittam/services/document/db"
)

// stubObjectStore is a no-op document.ObjectStore. PresignedPutURL records
// whether it was invoked so the test can assert a rejected upload never
// mints a presigned URL (the fix must run its folder check before the
// PresignedPutURL call, not just before the Document literal).
type stubObjectStore struct {
	presignedPutCalled bool
}

func (s *stubObjectStore) PresignedPutURL(_ context.Context, key string, _ time.Duration) (string, error) {
	s.presignedPutCalled = true
	return "https://storage.example.com/put/" + key, nil
}
func (s *stubObjectStore) PresignedGetURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.example.com/get/" + key, nil
}
func (s *stubObjectStore) Stat(_ context.Context, _ string) (int64, error) { return 0, nil }
func (s *stubObjectStore) Delete(_ context.Context, _ string) error        { return nil }

func TestDocument_InitiateUpload_RejectsFolderFromAnotherTenant(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	tenantA := seedDocumentIsolationTenant(t, pool, "Document Isolation Tenant A")
	tenantB := seedDocumentIsolationTenant(t, pool, "Document Isolation Tenant B")
	folderB := seedDocumentFolder(t, pool, tenantB)

	repo := documentdb.NewPostgres(pool)
	store := &stubObjectStore{}
	svc := document.NewService(repo, store, nil)

	uploadedBy := uuid.New()
	_, err := svc.InitiateUpload(ctx, &document.InitiateUploadRequest{
		TenantID:   tenantA,
		FolderID:   &folderB,
		Name:       "cross-tenant.pdf",
		MimeType:   "application/pdf",
		UploadedBy: uploadedBy,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, document.ErrFolderNotFound)
	assert.False(t, store.presignedPutCalled,
		"a rejected upload must not mint a presigned URL it will never use")

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM documents WHERE tenant_id = $1`, tenantA).Scan(&count))
	assert.Equal(t, 0, count, "no documents row must be created for tenant A")
}

// seedDocumentIsolationTenant inserts a tenant for the document isolation
// test and registers cleanup. country_code and primary_currency_code are
// NOT NULL since migration 014; the name must be unique under
// tenants_name_ci_unique (lower(trim(name))), so it is suffixed with a fresh
// UUID.
func seedDocumentIsolationTenant(t *testing.T, pool *pgxpool.Pool, label string) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, label+" "+tenantID.String(), "doc-iso-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM documents WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete documents")
		_, err = pool.Exec(ctx, `DELETE FROM folders WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete folders")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

// seedDocumentFolder inserts a folder owned by tenantID and returns its id.
// Cleanup is handled by seedDocumentIsolationTenant's tenant-scoped DELETE.
func seedDocumentFolder(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	folderID := uuid.New()
	createdBy := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO folders (id, tenant_id, name, created_by) VALUES ($1, $2, $3, $4)`,
		folderID, tenantID, "Cross-Tenant Folder", createdBy)
	require.NoError(t, err, "insert folder")
	return folderID
}
