// iam service entrypoint.
// Provides identity, authentication, authorisation, and multi-tenancy.
// All other services validate tokens via the IAM gRPC interceptor.
package main

import (
	"log"

	"github.com/wegofwd2020/thittam/pkg/server"
	"github.com/wegofwd2020/thittam/services/iam"
)

func main() {
	srv := server.New(server.Config{
		Name:        "iam",
		Port:        8086,
		MetricsPort: 9096,
	}, nil)

	// In production, all dependencies are wired from environment:
	//   repo   — postgres-backed repository (implements iam.Repository)
	//   authr  — auth.NewResolver(localProvider, oidcProvider, oidcConfigStore)
	//   tokens — jwt.NewIssuer(privateKey, redisClient)
	//   hasher — bcrypt.NewHasher()
	//   verifr — bcrypt.NewVerifier()
	//
	// For now, nil stubs allow startup without infrastructure.
	_ = iam.NewHandler(iam.NewService(nil, nil, nil, nil, nil))

	// Register gRPC service here when proto generation is set up:
	// iampb.RegisterIAMServiceServer(srv.GRPCServer(), handler)

	log.Printf("iam service ready on :8086")
	if err := srv.Run(); err != nil {
		log.Fatalf("iam: %v", err)
	}
}
