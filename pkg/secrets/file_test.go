package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSource_GetSecret_ReturnsFileContents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wantContent := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAK=\n-----END RSA PRIVATE KEY-----"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jwt_private.pem"), []byte(wantContent), 0600))

	src := NewFileSource(dir)
	got, err := src.GetSecret(context.Background(), "jwt_private.pem")

	require.NoError(t, err)
	assert.Equal(t, wantContent, string(got))
}

func TestFileSource_GetSecret_ReturnsErrSecretNotFoundForMissingFile(t *testing.T) {
	t.Parallel()

	src := NewFileSource(t.TempDir())
	_, err := src.GetSecret(context.Background(), "nonexistent.pem")

	assert.ErrorIs(t, err, ErrSecretNotFound)
}

func TestFileSource_GetSecret_ResolvesRelativeName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "db_password"), []byte("s3cr3t"), 0600))

	src := NewFileSource(dir)
	got, err := src.GetSecret(context.Background(), "db_password")

	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", string(got))
}

func TestFileSource_GetSecret_ContextIsIgnored(t *testing.T) {
	t.Parallel()
	// FileSource reads synchronously; context cancellation has no effect on the
	// happy path. This test documents the behaviour rather than asserting on it.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "key"), []byte("data"), 0600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	src := NewFileSource(dir)
	got, err := src.GetSecret(ctx, "key")

	require.NoError(t, err)
	assert.Equal(t, "data", string(got))
}
