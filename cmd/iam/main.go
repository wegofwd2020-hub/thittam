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
//  2. IAM_KEY_DIR points to a directory with gitignored PEM files.
//  3. FileSource reads the bytes — still never an env var containing the key.
//
// # Pending TODOs before production:
//
//   - Implement pkg/auth/jwt — JWTIssuer backed by Redis for refresh token storage
//   - Wire OIDCProvider and OIDCConfigStore for tenants with OIDC auth
//   - Pass jwtPrivateKey bytes to jwt.NewIssuer (not stored in env/log/file)
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/secrets"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("iam: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("iam: startup: ping database: %v", err)
	}

	repo := iamdb.NewPostgres(pool)

	// --- Select secret source based on environment ---
	//
	// VAULT_ADDR present → production: load T1 secrets from Vault.
	// VAULT_ADDR absent  → local dev: load T1 secrets from key files.
	vaultAddr := os.Getenv("VAULT_ADDR")

	var jwtPrivateKey []byte
	if vaultAddr != "" {
		vaultSrc := secrets.NewVaultSource(secrets.VaultConfig{
			Address:  vaultAddr,
			Mount:    getenv("VAULT_KV_MOUNT", "secret"),
			RoleID:   requireenv("VAULT_ROLE_ID"),
			SecretID: requireenv("VAULT_SECRET_ID"),
		})
		if err := waitForVault(ctx, vaultSrc, 30*time.Second); err != nil {
			log.Fatalf("iam: startup: %v", err)
		}
		jwtPrivateKey, err = vaultSrc.GetSecret(ctx, "iam/jwt-private-key")
		if err != nil {
			log.Fatalf("iam: startup: load JWT private key from Vault: %v", err)
		}
	} else {
		keyDir := getenv("IAM_KEY_DIR", "./keys")
		fileSrc := secrets.NewFileSource(keyDir)
		jwtPrivateKey, err = fileSrc.GetSecret(ctx, "jwt_private.pem")
		if err != nil {
			log.Fatalf("iam: startup: load JWT private key from %s: %v\n"+
				"  Run: openssl genrsa -out keys/jwt_private.pem 2048", keyDir, err)
		}
	}

	// jwtPrivateKey bytes are held in memory only. Never stored in an env var,
	// log, or written back to a file.

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer rdb.Close()

	// --- JWT issuer (T1 secret consumed here, bytes not retained) ---
	tokenIssuer, err := auth.NewJWTIssuer(jwtPrivateKey, rdb, auth.JWTConfig{})
	if err != nil {
		log.Fatalf("iam: startup: create JWT issuer: %v", err)
	}

	// --- Auth stack ---
	// New passwords are hashed with argon2id (OWASP-minimum params).
	// Existing bcrypt hashes remain valid and are silently upgraded to argon2id
	// on the user's next successful login via Service.rehashIfNeeded.
	hasher := auth.NewArgon2idHasher()
	verifier := auth.NewDualVerifier()
	localProvider := auth.NewLocalProvider(repo, repo, verifier)

	// TODO: wire OIDCProvider + OIDCConfigStore for tenants using OIDC auth.
	// For now all tenants use local auth. resolver.Authenticate falls back to
	// localProvider when no authorization code is present in the request.
	resolver := auth.NewResolver(localProvider, nil, nil)

	// --- Build service ---
	svc := iam.NewService(repo, resolver, tokenIssuer, hasher, verifier)
	handler := iam.NewHandler(svc)

	// --- gRPC server ---
	srv := server.New(server.Config{
		Name:        "iam",
		Port:        8086,
		MetricsPort: 9096,
	}, nil)

	iamv1.RegisterIAMServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})

	if vaultAddr != "" {
		log.Printf("iam service ready on :8086")
	} else {
		log.Printf("iam service ready on :8086 (local dev — no Vault)")
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("iam: %v", err)
	}
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// waitForVault blocks until CheckHealth succeeds or the deadline is exceeded.
func waitForVault(ctx context.Context, src *secrets.VaultSource, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := src.CheckHealth(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return src.CheckHealth(ctx)
		}
		log.Printf("iam: waiting for Vault (%s remaining)...", time.Until(deadline).Round(time.Second))
		time.Sleep(2 * time.Second)
	}
}

// requireenv returns the value of an env var or fatals if it is empty.
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
