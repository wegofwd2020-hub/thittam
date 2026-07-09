// project-management service entrypoint.
// Registers the vertical interceptor and project service handlers.
//
// The vertical interceptor injects the tenant's vertical config into the gRPC
// context on every request, allowing service methods to validate phase types
// and return industry-specific labels without database round-trips (Redis cache).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	projectv1 "github.com/wegofwd2020/thittam/gen/project/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/corsutil"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/iamclient"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	verticaldb "github.com/wegofwd2020/thittam/pkg/vertical/db"
	"github.com/wegofwd2020/thittam/services/project"
	projectdb "github.com/wegofwd2020/thittam/services/project/db"
)

func main() {
	ctx := context.Background()

	// --- Database ---
	dbURL := requireenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("project-management: startup: connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("project-management: startup: ping database: %v", err)
	}

	// --- Redis ---
	redisURL := requireenv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	defer func() { _ = rdb.Close() }()

	// --- Vertical config loader ---
	// The loader provides a Redis-cached (5-min TTL) view of the tenant's
	// vertical configuration. It injects the config into the gRPC context
	// via the server's UnaryInterceptor / StreamInterceptor.
	vdb := verticaldb.NewStore(pool)
	loader := vertical.NewLoader(rdb, vdb, nil) // nil → uses default logger

	// --- NATS JetStream ---
	natsURL := requireenv("NATS_URL")
	nc, err := nats.Connect(natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Fatalf("project-management: startup: connect to NATS: %v", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("project-management: startup: create JetStream context: %v", err)
	}
	pub := jetstream.NewPublisher(js)

	// --- IAM permission checker (ADR-014 Phase 2) ---
	iamPerm, closeIAM, err := iamclient.DialFromEnv("project-management")
	if err != nil {
		log.Fatalf("project-management: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()

	// --- Repository and service ---
	repo := projectdb.NewPostgres(pool)
	svc := project.NewService(repo, &projectPublisher{pub: pub})
	handler := project.NewHandler(svc)
	if iamPerm != nil {
		handler = handler.WithPermissionChecker(iamPerm)
	}

	// --- gRPC server ---
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("project-management: startup: load JWT public key: %v", err)
	}
	srv := server.New(server.Config{
		Name: "project-management",
		// Ports shifted from 8080/9090 to 8090/9100 to avoid conflicts with
		// other projects on the same dev host (notably StudyBuddy, which
		// uses 8080 for its API). All other thittam services keep 808x/909x.
		Port:        8090,
		MetricsPort: 9100,
		Loader:      loader,
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
	}, nil)

	projectv1.RegisterProjectServiceServer(srv.GRPCServer(), handler)

	srv.RegisterHealthChecker("postgres", &dbChecker{pool: pool})
	srv.RegisterHealthChecker("nats", &natsChecker{nc: nc})
	srv.RegisterHealthChecker("redis", &redisChecker{rdb: rdb})

	// --- REST gateway (grpc-gateway, ADR-014 follow-up #60) ---
	// UI calls REST endpoints like GET /api/v1/productions. The generated mux
	// lives on :9080 — a dedicated port parallel to IAM's 9086 pattern. The
	// original 9090 slot is taken by a neighbouring process on this host
	// (see Port=8090 comment above for the same 8080→8090 rationale).
	go func() {
		// x-caller-* and x-tenant-id are deliberately NOT forwarded: identity
		// comes from the verified token (#138), and forwarding them would let a
		// browser assert its own role. X-Project-Id selects a resource, not an
		// identity. Authorization arrives without a matcher (permanent header).
		headerMatcher := func(key string) (string, bool) {
			if key == "X-Project-Id" {
				return key, true
			}
			return runtime.DefaultHeaderMatcher(key)
		}
		gwMux := runtime.NewServeMux(
			runtime.WithIncomingHeaderMatcher(headerMatcher),
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
		if err := projectv1.RegisterProjectServiceHandlerFromEndpoint(ctx, gwMux, "localhost:8090", opts); err != nil {
			log.Fatalf("project-management: register gateway: %v", err)
		}
		extraOrigins := corsutil.ExtraOriginsFromEnv()
		corsHandler := cors.New(cors.Options{
			AllowOriginFunc: corsutil.OriginFunc(extraOrigins...),
			AllowedMethods: []string{
				http.MethodGet, http.MethodPost, http.MethodPut,
				http.MethodPatch, http.MethodDelete, http.MethodOptions,
			},
			AllowedHeaders: []string{
				"Content-Type", "Authorization", "Accept", "X-Project-Id",
			},
			AllowCredentials: true,
		}).Handler(gwMux)
		log.Printf("project-management REST gateway ready on :9080 (CORS: local-dev + %d extra origin(s))", len(extraOrigins))
		if err := http.ListenAndServe(":9080", corsHandler); err != nil {
			log.Fatalf("project-management: gateway listen: %v", err)
		}
	}()

	log.Printf("project-management service ready on :8090")
	if err := srv.Run(); err != nil {
		log.Fatalf("project-management: %v", err)
	}
}

// dbChecker implements observability.HealthChecker for the PostgreSQL pool.
type dbChecker struct{ pool *pgxpool.Pool }

func (c *dbChecker) CheckHealth(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// natsChecker implements observability.HealthChecker for the NATS connection.
// /readyz returns 503 when NATS is unreachable so Kubernetes stops routing
// traffic to a pod that cannot publish domain events.
type natsChecker struct{ nc *nats.Conn }

func (c *natsChecker) CheckHealth(_ context.Context) error {
	if !c.nc.IsConnected() {
		return nats.ErrConnectionClosed
	}
	return nil
}

// redisChecker implements observability.HealthChecker for the Redis connection.
// /readyz returns 503 when Redis is unreachable — the vertical config cache
// cannot be populated, which would cause all tenant-context lookups to fail.
type redisChecker struct{ rdb *redis.Client }

func (c *redisChecker) CheckHealth(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// requireenv returns the value of an env var or fatals if it is empty.
func requireenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("project-management: startup: required environment variable %s is not set", key)
	}
	return v
}

// projectPublisher adapts *jetstream.Publisher to project.EventPublisher.
type projectPublisher struct{ pub *jetstream.Publisher }

func (p *projectPublisher) PublishProductionCreated(ctx context.Context, prod *project.Production) error {
	return p.pub.Publish(ctx, events.SubjectProjectCreated, prod.TenantID, events.ProjectCreatedPayload{
		ProjectID: prod.ID,
		Title:     prod.Title,
		Status:    prod.Status,
		StartDate: prod.StartDate,
	})
}

func (p *projectPublisher) PublishCrewMemberAdded(ctx context.Context, c *project.CrewMember) error {
	return p.pub.Publish(ctx, events.SubjectProjectMemberAssigned, c.TenantID, events.ProjectMemberAssignedPayload{
		ProjectID: c.ProductionID,
		// MemberCount requires a separate query; publish with 0 for now — consumers
		// should treat this as a "member added" signal and re-query if needed.
		MemberCount: 0,
	})
}
