package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/wegofwd2020/thittam/pkg/corsutil"
)

// GatewayConfig configures a REST-over-gRPC gateway for one service.
type GatewayConfig struct {
	ServiceName  string // for log lines, e.g. "budget-planning"
	GRPCEndpoint string // the service's own gRPC listen address, e.g. "localhost:8081"
	HTTPPort     int    // the gateway's HTTP listen port, e.g. 9081

	// Register is the generated Register<Svc>ServiceHandlerFromEndpoint.
	Register func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error

	// ProjectHeader, when true, forwards the X-Project-Id request header into
	// gRPC metadata and adds it to the CORS allowed-headers. Identity always
	// comes from the verified token (#138); X-Project-Id selects a resource.
	ProjectHeader bool

	// Wrap, when non-nil, wraps the gateway mux with outer middleware (inside
	// CORS). iam uses it to rate-limit /api/v1/auth/*.
	Wrap func(http.Handler) http.Handler
}

// buildGatewayHandler assembles the gateway mux, optional outer middleware, and
// CORS. It does not listen. Register dials lazily (the gRPC endpoint is not
// contacted until the first RPC), so this is safe to call in tests.
func buildGatewayHandler(ctx context.Context, cfg GatewayConfig) (http.Handler, error) {
	var muxOpts []runtime.ServeMuxOption
	if cfg.ProjectHeader {
		matcher := func(key string) (string, bool) {
			if key == "X-Project-Id" {
				return key, true
			}
			return runtime.DefaultHeaderMatcher(key)
		}
		muxOpts = append(muxOpts, runtime.WithIncomingHeaderMatcher(matcher))
	}
	muxOpts = append(muxOpts, runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions: protojson.MarshalOptions{
			UseProtoNames:   true,
			EmitUnpopulated: true,
		},
		UnmarshalOptions: protojson.UnmarshalOptions{
			DiscardUnknown: true,
		},
	}))
	mux := runtime.NewServeMux(muxOpts...)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := cfg.Register(ctx, mux, cfg.GRPCEndpoint, opts); err != nil {
		return nil, fmt.Errorf("%s: register gateway: %w", cfg.ServiceName, err)
	}

	var h http.Handler = mux
	if cfg.Wrap != nil {
		h = cfg.Wrap(h)
	}

	allowedHeaders := []string{"Content-Type", "Authorization", "Accept"}
	if cfg.ProjectHeader {
		allowedHeaders = append(allowedHeaders, "X-Project-Id")
	}
	corsHandler := cors.New(cors.Options{
		AllowOriginFunc: corsutil.OriginFunc(corsutil.ExtraOriginsFromEnv()...),
		AllowedMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		AllowedHeaders:   allowedHeaders,
		AllowCredentials: true,
	}).Handler(h)
	return corsHandler, nil
}

// RunRESTGateway builds the gateway handler and serves it on :HTTPPort. It
// blocks; callers launch it with `go`.
func RunRESTGateway(ctx context.Context, cfg GatewayConfig) error {
	handler, err := buildGatewayHandler(ctx, cfg)
	if err != nil {
		return err
	}
	log.Printf("%s REST gateway ready on :%d", cfg.ServiceName, cfg.HTTPPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", cfg.HTTPPort), handler)
}
