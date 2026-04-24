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
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/cors"
	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/corsutil"
	appcrypto "github.com/wegofwd2020/thittam/pkg/crypto"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/migrate"
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

	var (
		jwtPrivateKey   []byte
		oidcEncryptKey  []byte
	)
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
		oidcEncryptKey, err = vaultSrc.GetSecret(ctx, "iam/oidc-encryption-key")
		if err != nil {
			log.Fatalf("iam: startup: load OIDC encryption key from Vault: %v", err)
		}
	} else {
		keyDir := getenv("IAM_KEY_DIR", "./keys")
		fileSrc := secrets.NewFileSource(keyDir)
		jwtPrivateKey, err = fileSrc.GetSecret(ctx, "jwt_private.pem")
		if err != nil {
			log.Fatalf("iam: startup: load JWT private key from %s: %v\n"+
				"  Run: openssl genrsa -out keys/jwt_private.pem 2048", keyDir, err)
		}
		oidcEncryptKey, err = fileSrc.GetSecret(ctx, "oidc_encryption.key")
		if err != nil {
			log.Fatalf("iam: startup: load OIDC encryption key from %s: %v\n"+
				"  Run: openssl rand -out keys/oidc_encryption.key 32", keyDir, err)
		}
	}
	if len(oidcEncryptKey) != 32 {
		log.Fatalf("iam: startup: oidc encryption key must be exactly 32 bytes, got %d", len(oidcEncryptKey))
	}

	// T1 key bytes are held in memory only. Never stored in env vars, logs, or files.

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer func() { _ = rdb.Close() }()

	// --- JWT issuer ---
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

	// OIDC provider: decryptingOIDCStore wraps repo.GetOIDCConfig and decrypts
	// the AES-256-GCM client secret before passing it to the exchanger.
	exchanger := auth.NewHTTPExchanger()
	oidcStore := &decryptingOIDCStore{inner: repo, key: oidcEncryptKey}
	oidcProvider := auth.NewOIDCProvider(oidcStore, exchanger, repo, repo)
	resolver := auth.NewResolver(localProvider, oidcProvider, oidcStore)

	// --- Schema migrator ---
	// MIGRATIONS_BASE_PATH points to the root of the migrations directory tree.
	// Each subdirectory (shared, iam, project, …) is applied in dependency order
	// when a new tenant is provisioned. All paths must be relative to the working
	// directory of the running binary (set via Kubernetes/Docker workdir or Makefile).
	migrationsBase := getenv("MIGRATIONS_BASE_PATH", "migrations")
	migrationPaths := []string{
		migrationsBase + "/shared",
		migrationsBase + "/iam",
		migrationsBase + "/project",
		migrationsBase + "/budget",
		migrationsBase + "/expense",
		migrationsBase + "/ledger",
		migrationsBase + "/inventory",
		migrationsBase + "/notifications",
		migrationsBase + "/document",
		migrationsBase + "/reporting",
		migrationsBase + "/billing",
		migrationsBase + "/audit",
	}
	schemaMigrator := &iamSchemaMigrator{
		m: migrate.New(migrate.Options{
			DBURL:       dbURL,
			Parallelism: 1, // single-tenant path — no benefit from >1 worker
		}),
		paths: migrationPaths,
	}

	// --- Build service ---
	svc := iam.NewService(repo, resolver, tokenIssuer, hasher, verifier).
		WithOIDCEncryptionKey(oidcEncryptKey).
		WithSchemaMigrator(schemaMigrator)
	handler := iam.NewHandler(svc)

	// --- Background: impersonation session expiry ticker ---
	// Runs every minute. Sessions whose expires_at has passed but ended_at is
	// still NULL are closed. This bounds how long a stale impersonation can
	// remain open if the caller never called EndImpersonation explicitly.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			n, err := repo.ExpireImpersonationSessions(ctx)
			if err != nil {
				log.Printf("iam: expire impersonation sessions: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("iam: expired %d impersonation session(s)", n)
			}
		}
	}()

	// --- gRPC server ---
	// UnaryCallerInterceptor reads Kong-injected metadata (x-caller-id,
	// x-caller-role, x-caller-email, x-forwarded-for) and populates the
	// caller identity + audit actor in each request context. Admin RPCs
	// (SetOIDCConfig, SuspendTenant, DeactivateUser) call RequireRole to
	// enforce platform_admin access at the handler level.
	srv := server.New(server.Config{
		Name:        "iam",
		Port:        8086,
		MetricsPort: 9096,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryCallerInterceptor()},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamCallerInterceptor()},
	}, nil)

	iamv1.RegisterIAMServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})

	// --- REST gateway (grpc-gateway, ADR-014 follow-up #60) ---
	// The UI calls REST endpoints (/api/v1/auth/login etc.). Mount the
	// generated grpc-gateway mux on a separate HTTP port so the gRPC port
	// stays a clean gRPC-only surface for service-to-service calls.
	go func() {
		// UseProtoNames=true emits snake_case field names (matching the .proto)
		// instead of grpc-gateway's default camelCase. The web client's TS types
		// are snake_case (e.g. TokenPair.access_token), so this avoids per-field
		// rename work on the UI side.
		gwMux := runtime.NewServeMux(
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
				MarshalOptions: protojson.MarshalOptions{
					UseProtoNames:   true,
					EmitUnpopulated: true,
				},
				UnmarshalOptions: protojson.UnmarshalOptions{
					DiscardUnknown: true,
				},
			}),
		)
		opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		if err := iamv1.RegisterIAMServiceHandlerFromEndpoint(ctx, gwMux, "localhost:8086", opts); err != nil {
			log.Fatalf("iam: register gateway: %v", err)
		}
		// CORS: accepts the Next.js dev server on :3100/:3000 from any
		// loopback or RFC-1918 host (local dev, LAN demos) plus any exact
		// origin listed in CORS_EXTRA_ORIGINS (pre-Kong cloud deploys).
		// Production CORS is handled by Kong at the edge.
		extraOrigins := corsutil.ExtraOriginsFromEnv()
		corsHandler := cors.New(cors.Options{
			AllowOriginFunc: corsutil.OriginFunc(extraOrigins...),
			AllowedMethods: []string{
				http.MethodGet, http.MethodPost, http.MethodPut,
				http.MethodPatch, http.MethodDelete, http.MethodOptions,
			},
			AllowedHeaders: []string{
				"Content-Type", "Authorization",
				"X-Tenant-Id", "X-Project-Id",
				"X-Caller-Id", "X-Caller-Email", "X-Caller-Role",
			},
			AllowCredentials: true,
		}).Handler(gwMux)
		log.Printf("iam REST gateway ready on :9086 (CORS: local-dev + %d extra origin(s))", len(extraOrigins))
		if err := http.ListenAndServe(":9086", corsHandler); err != nil {
			log.Fatalf("iam: gateway listen: %v", err)
		}
	}()

	if vaultAddr != "" {
		log.Printf("iam service ready on :8086")
	} else {
		log.Printf("iam service ready on :8086 (local dev — no Vault)")
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("iam: %v", err)
	}
}

// decryptingOIDCStore wraps auth.OIDCConfigStore and decrypts the AES-256-GCM
// client secret before returning the config to the caller.
type decryptingOIDCStore struct {
	inner auth.OIDCConfigStore
	key   []byte
}

func (s *decryptingOIDCStore) GetOIDCConfig(ctx context.Context, tenantID uuid.UUID) (*auth.OIDCConfig, error) {
	cfg, err := s.inner.GetOIDCConfig(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	plaintext, err := appcrypto.Decrypt(s.key, cfg.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("iam: decrypt client secret for tenant %s: %w", tenantID, err)
	}
	cfg.ClientSecret = plaintext
	return cfg, nil
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// redisChecker implements observability.HealthChecker for the Redis client.
// /readyz returns 503 until Redis is reachable — the JWT issuer and vertical
// config cache both depend on it, so the service must not accept traffic without it.
type redisChecker struct{ rdb *redis.Client }

func (c *redisChecker) CheckHealth(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
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

// iamSchemaMigrator implements iam.SchemaMigrator by running every migration
// subdirectory in paths against the new tenant schema, in order.
type iamSchemaMigrator struct {
	m     *migrate.ParallelMigrator
	paths []string
}

func (s *iamSchemaMigrator) MigrateTenantSchema(ctx context.Context, tenantID uuid.UUID) error {
	for _, path := range s.paths {
		if err := s.m.MigrateTenant(ctx, tenantID, path); err != nil {
			return fmt.Errorf("migrate schema %s for tenant %s: %w", path, tenantID, err)
		}
	}
	return nil
}
