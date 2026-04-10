// Package secrets provides a tiered abstraction for loading secrets at service
// startup.
//
// # T1 vs config distinction (Rule #2 clarification)
//
// The contradiction between CODING_RULES.md Rule #2 ("secrets from env vars")
// and the security policy ("T1 data never in env vars") is resolved here:
//
//   - T1 secrets (JWT signing keys, database passwords, API master keys) are
//     fetched from Vault using a VaultSource. They are held in memory and never
//     appear in environment variables, /proc/environ, container inspect output,
//     or log lines.
//
//   - Non-sensitive config (service ports, endpoint URLs, feature flags) and
//     Vault AppRole credentials (limited-privilege, not the secret itself) are
//     read from environment variables per Rule #2.
//
//   - Local development uses a FileSource: env vars carry a *file path*
//     (e.g. JWT_PRIVATE_KEY_PATH=./keys/jwt_private.pem), and the secret bytes
//     are read from the file. The file is gitignored.
//
// # Data classification
//
//	Tier  Examples                       Source
//	────  ─────────────────────────────  ──────────────────────
//	T1    JWT private key, DB password   VaultSource (Vault KV)
//	T2    SendGrid API key, Twilio token VaultSource (or K8s Secret → env)
//	T3    S3 endpoint, service ports     env var (os.Getenv)
//	T4    Feature flags, log level       env var or config map
//
// # Usage
//
//	src := secrets.NewVaultSource(secrets.VaultConfig{
//	    Address:  os.Getenv("VAULT_ADDR"),
//	    RoleID:   os.Getenv("VAULT_ROLE_ID"),    // AppRole credentials are T3
//	    SecretID: os.Getenv("VAULT_SECRET_ID"),  // AppRole credentials are T3
//	    Mount:    "secret",
//	})
//
//	keyBytes, err := src.GetSecret(ctx, "iam/jwt-private-key")
//	if err != nil { log.Fatalf("startup: load JWT key: %v", err) }
package secrets

import (
	"context"
	"errors"
)

// ErrSecretNotFound is returned when the secret path exists but the named key
// is absent from the Vault KV response.
var ErrSecretNotFound = errors.New("secrets: secret not found")

// Source loads a named secret and returns its raw bytes.
// Implementations must be safe for concurrent use.
type Source interface {
	// GetSecret returns the raw bytes for the named secret.
	// name is the logical secret path, e.g. "iam/jwt-private-key".
	// Returns ErrSecretNotFound if the path or key does not exist.
	GetSecret(ctx context.Context, name string) ([]byte, error)
}
