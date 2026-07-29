package iamclient

import (
	"fmt"
	"log"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
)

// EnvAddr is the env var that holds the IAM service gRPC address.
// T3 (non-sensitive endpoint) — safe as a plain env var per CODING_RULES rule 2.
const EnvAddr = "IAM_SERVICE_ADDR"

// DialFromEnv connects to the IAM service via gRPC and returns a configured
// PermissionChecker, plus a close function the caller should defer.
//
// If IAM_SERVICE_ADDR is unset, returns (nil, no-op close, nil) — a nil
// checker. The caller decides what that means: gated RPCs fail closed with
// Internal (#138), so a service that requires authz should Fatalf on a nil
// checker rather than start.
//
// Inside the Istio service mesh we use plain credentials; Envoy sidecars
// terminate mTLS. Match the billing→document pattern in cmd/billing/main.go.
func DialFromEnv(serviceName string) (*PermissionChecker, func() error, error) {
	addr := os.Getenv(EnvAddr)
	if addr == "" {
		log.Printf("%s: %s unset — no IAM permission checker; gated RPCs fail closed with Internal (#138). A service that requires authz should refuse to start.", serviceName, EnvAddr)
		return nil, func() error { return nil }, nil
	}

	// ForwardAuthUnaryClientInterceptor forwards the caller's bearer token so
	// iam verifies the same token.
	//
	// REQUIRED, not defence in depth. CheckPermission is no longer in
	// interceptor.PublicMethods (#139 slice I), so iam rejects an anonymous
	// call. Without this interceptor every permission-gated RPC in this
	// service fails closed — do not remove it.
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(interceptor.ForwardAuthUnaryClientInterceptor()),
	)
	if err != nil {
		return nil, func() error { return nil }, fmt.Errorf("iamclient: dial %s: %w", addr, err)
	}

	checker := NewPermissionChecker(iamv1.NewIAMServiceClient(conn))
	log.Printf("%s: IAM permission checks ENABLED (target=%s)", serviceName, addr)
	return checker, conn.Close, nil
}
