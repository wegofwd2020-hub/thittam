package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FileSource loads secrets from files on the local filesystem.
// It is intended for local development only. In production, use VaultSource.
//
// The typical local-dev pattern:
//
//  1. An env var carries the file *path* (not the secret):
//     JWT_PRIVATE_KEY_PATH=./keys/jwt_private.pem
//
//  2. FileSource reads the bytes from that path:
//     src := secrets.NewFileSource("./keys")
//     keyBytes, err := src.GetSecret(ctx, "jwt_private.pem")
//
// The key files must be gitignored. Never commit private keys.
type FileSource struct {
	// baseDir is the directory from which relative secret names are resolved.
	baseDir string
}

// NewFileSource creates a FileSource rooted at baseDir.
// name arguments passed to GetSecret are joined with baseDir.
func NewFileSource(baseDir string) *FileSource {
	return &FileSource{baseDir: baseDir}
}

// GetSecret reads the file at baseDir/name and returns its contents.
// Returns ErrSecretNotFound if the file does not exist.
func (f *FileSource) GetSecret(_ context.Context, name string) ([]byte, error) {
	path := filepath.Join(f.baseDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, path)
		}
		return nil, fmt.Errorf("secrets: read file %s: %w", path, err)
	}
	return data, nil
}

// Compile-time interface check.
var _ Source = (*FileSource)(nil)
