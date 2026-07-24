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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	nats "github.com/nats-io/nats.go"
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
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/pkg/ratelimit"
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

	// --- NATS JetStream: billing consumer (#118) ---
	// iam's first JetStream consumer. Subscribes to the FINANCIAL stream
	// filtered to thittam.billing.subscription.suspended and mirrors the
	// suspension onto the tenant, starting the retention clock. NATS_URL is
	// T3 (service endpoint) config; if it is unset iam still boots — the
	// consumer is simply disabled, matching local dev without a NATS server.
	// NATS powers ONLY the best-effort billing consumer — it is NOT on iam's
	// critical auth path (login / JWT issuance / RBAC / token validation). iam is
	// the identity SPOF, so a NATS outage must never take it down or pull it from
	// rotation: connect/subscribe failures are logged and the consumer is disabled
	// (not fatal), and NATS deliberately does NOT gate /readyz (unlike other
	// services). The suspend event just won't be consumed until NATS recovers.
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		if nc, err := nats.Connect(natsURL,
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		); err != nil {
			log.Printf("iam: WARN connect to NATS failed — billing consumer disabled: %v", err)
		} else if js, err := nc.JetStream(); err != nil {
			log.Printf("iam: WARN JetStream context failed — billing consumer disabled: %v", err)
			_ = nc.Drain()
		} else if billingSub, err := jetstream.Subscribe(js,
			jetstream.StreamFinancial,
			jetstream.ConsumerIamBilling,
			iam.NewBillingConsumer(svc).Handle,
		); err != nil {
			log.Printf("iam: WARN subscribe to billing consumer failed — disabled: %v", err)
			_ = nc.Drain()
		} else {
			defer func() { _ = billingSub.Unsubscribe() }()
			defer func() { _ = nc.Drain() }()
			log.Printf("iam: billing consumer subscribed (FINANCIAL/%s)", jetstream.ConsumerIamBilling)
		}
	} else {
		log.Printf("iam: NATS_URL not set — billing consumer disabled (tenant suspension on subscription.suspended will not fire)")
	}

	// --- gRPC server ---
	// UnaryAuthInterceptor verifies the caller's JWT (#138) and populates the
	// caller identity + audit actor in each request context from verified
	// claims only. Admin RPCs (SetOIDCConfig, SuspendTenant, DeactivateUser)
	// call RequireRole to enforce platform_admin access at the handler level.
	jwtVerifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("iam: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name:        "iam",
		Port:        8086,
		MetricsPort: 9096,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(jwtVerifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(jwtVerifier, interceptor.PublicMethods)},
	}, nil)

	iamv1.RegisterIAMServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})
	// NATS is intentionally NOT a readiness checker: it backs only the best-effort
	// billing consumer, and iam (the auth SPOF) must stay ready during a NATS outage.

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
		// Rate-limit /api/v1/auth/* by client IP to blunt credential stuffing.
		// Defaults to 10 attempts per IP per minute; override with
		// AUTH_RATE_LIMIT and AUTH_RATE_WINDOW env vars. Fails open if Redis
		// is unreachable — the limiter is a safety net, not an auth layer.
		authRateLimiter := ratelimit.Middleware(rdb, ratelimit.Config{
			Limit:     envInt("AUTH_RATE_LIMIT", 10),
			Window:    envDuration("AUTH_RATE_WINDOW", time.Minute),
			KeyPrefix: "iam:auth",
		})
		// Wrap only /api/v1/auth/* with the limiter; everything else goes
		// straight to the grpc-gateway mux.
		routed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
				authRateLimiter(gwMux).ServeHTTP(w, r)
				return
			}
			gwMux.ServeHTTP(w, r)
		})

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
				"Content-Type", "Authorization", "Accept",
			},
			AllowCredentials: true,
		}).Handler(routed)
		log.Printf("iam REST gateway ready on :9086 (CORS: local-dev + %d extra origin(s); auth rate-limit: %d/%s)", len(extraOrigins), envInt("AUTH_RATE_LIMIT", 10), envDuration("AUTH_RATE_WINDOW", time.Minute))
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

// envInt reads an int env var or returns the default. Invalid values are
// treated as unset (warn in logs so misconfiguration is visible).
func envInt(key string, defaultValue int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("iam: env %s=%q is not an int, using default %d", key, raw, defaultValue)
		return defaultValue
	}
	return n
}

// envDuration reads a Go duration env var (e.g. "1m", "30s") or returns the default.
func envDuration(key string, defaultValue time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("iam: env %s=%q is not a duration, using default %s", key, raw, defaultValue)
		return defaultValue
	}
	return d
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
