// iam service entrypoint.
// Provides identity, authentication, authorisation, and multi-tenancy.
// All other services validate tokens via the IAM gRPC interceptor.
//
// # Secret loading strategy (Rule #2 / security policy reconciliation)
//
// T1 secrets (JWT signing keys) are NEVER read from environment variables.
// The env var leak surface includes /proc/<pid>/environ, `docker inspect`,
// container orchestrator audit logs, and shell history.
//
// Production startup:
//  1. Read VAULT_ADDR, VAULT_ROLE_ID, VAULT_SECRET_ID from env (T3 config — acceptable).
//  2. Create a VaultSource and call CheckHealth — service refuses to mark itself
//     ready (/readyz returns 503) until Vault is reachable.
//  3. Fetch "iam/jwt-private-key" from Vault KV v2. Hold bytes in memory.
//  4. Pass the key bytes to the JWT issuer constructor. The bytes are never
//     written back to a file, log, or env var.
//
// Local development:
//  1. Set VAULT_ADDR="" to select the FileSource path.
//  2. JWT_PRIVATE_KEY_PATH points to a gitignored PEM file.
//  3. FileSource reads the bytes — still never an env var containing the key.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/wegofwd2020/thittam/pkg/secrets"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/iam"
)

func main() {
	ctx := context.Background()

	// --- Select secret source based on environment ---
	//
	// VAULT_ADDR present → production: load T1 secrets from Vault.
	// VAULT_ADDR absent  → local dev: load T1 secrets from key files.
	var src secrets.Source
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr != "" {
		// Production path: AppRole credentials are T3 config — acceptable as
		// env vars. They grant read-only access to a scoped set of secret paths.
		vaultSrc := secrets.NewVaultSource(secrets.VaultConfig{
			Address:  vaultAddr,
			Mount:    getenv("VAULT_KV_MOUNT", "secret"),
			RoleID:   requireenv("VAULT_ROLE_ID"),
			SecretID: requireenv("VAULT_SECRET_ID"),
		})
		src = vaultSrc

		// Register Vault connectivity as a /readyz dependency. The service will
		// return HTTP 503 until Vault is reachable and the token is obtained.
		// This prevents traffic from reaching the service before T1 secrets are loaded.
		srv := server.New(server.Config{
			Name:        "iam",
			Port:        8086,
			MetricsPort: 9096,
		}, nil)
		srv.Health().RegisterChecker("vault", vaultSrc)

		// Block startup until Vault is reachable. Retry for up to 30 seconds
		// to handle transient network delays during pod scheduling.
		if err := waitForVault(ctx, vaultSrc, 30*time.Second); err != nil {
			log.Fatalf("iam: startup: %v", err)
		}

		// Load T1 secrets. Fail fast if unavailable — do not start without them.
		jwtPrivateKey, err := src.GetSecret(ctx, "iam/jwt-private-key")
		if err != nil {
			log.Fatalf("iam: startup: load JWT private key from Vault: %v", err)
		}
		// jwtPrivateKey bytes are passed directly to the JWT issuer.
		// They are never stored in an env var, written to a log, or persisted.
		_ = jwtPrivateKey // TODO: pass to jwt.NewIssuer(jwtPrivateKey, redisClient)

		// Wire remaining dependencies from T3/T4 env vars (safe):
		//   DATABASE_URL    → postgres.NewPool(os.Getenv("DATABASE_URL"))
		//   REDIS_URL       → redis.NewClient(os.Getenv("REDIS_URL"))
		_ = iam.NewHandler(iam.NewService(nil, nil, nil, nil, nil))

		// Register gRPC service here when proto generation is set up:
		// iampb.RegisterIAMServiceServer(srv.GRPCServer(), handler)

		log.Printf("iam service ready on :8086")
		if err := srv.Run(); err != nil {
			log.Fatalf("iam: %v", err)
		}

	} else {
		// Local development path: key files are gitignored and loaded via FileSource.
		// The env var carries only a *directory path*, not the secret itself.
		keyDir := getenv("IAM_KEY_DIR", "./keys")
		src = secrets.NewFileSource(keyDir)

		jwtPrivateKey, err := src.GetSecret(ctx, "jwt_private.pem")
		if err != nil {
			log.Fatalf("iam: startup: load JWT private key from %s: %v\n"+
				"  Run: openssl genrsa -out keys/jwt_private.pem 2048", keyDir, err)
		}
		_ = jwtPrivateKey // TODO: pass to jwt.NewIssuer(jwtPrivateKey, redisClient)

		srv := server.New(server.Config{
			Name:        "iam",
			Port:        8086,
			MetricsPort: 9096,
		}, nil)

		_ = iam.NewHandler(iam.NewService(nil, nil, nil, nil, nil))

		log.Printf("iam service ready on :8086 (local dev — no Vault)")
		if err := srv.Run(); err != nil {
			log.Fatalf("iam: %v", err)
		}
	}
}

// waitForVault blocks until CheckHealth succeeds or the deadline is exceeded.
// This ensures the service does not start accepting traffic before T1 secrets
// are loaded.
func waitForVault(ctx context.Context, src *secrets.VaultSource, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := src.CheckHealth(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return src.CheckHealth(ctx) // return the actual error
		}
		log.Printf("iam: waiting for Vault (%s remaining)...", time.Until(deadline).Round(time.Second))
		time.Sleep(2 * time.Second)
	}
}

// requireenv returns the value of an env var or fatals if it is empty.
// Used only for T3 config that is required for startup — never for T1 secrets.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("iam: startup: required environment variable %s is not set", key)
	}
	return v
}

// getenv returns the value of an env var or a default if it is empty.
func getenv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
