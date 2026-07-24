package interceptor

import iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"

// Reflection method names have no generated constants — the grpc-go reflection
// package does not export them. They are only reachable when a server is built
// with Config.EnableReflection (default false).
const (
	reflectionV1      = "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"
	reflectionV1Alpha = "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"
)

// PublicMethods maps a fully-qualified gRPC method to the reason it may be
// called without a valid access token.
//
// An entry here is a deliberate, reviewable decision. Silence in this map means
// "authentication required" — that is the point of the map's existence, and why
// it is keyed on generated constants rather than string literals.
//
// Deliberately ABSENT, and why:
//
//	Logout               — carries a refresh token in the body, but the caller
//	                       holds an access token. Requiring it costs nothing and
//	                       closes an unauthenticated write.
//	HandlePaymentWebhook — nothing routes to it (no google.api.http annotation),
//	                       so failing closed breaks nothing. When webhooks ship
//	                       they need gateway signature verification, not a JWT (#139).
//	GetCurrentUser       — the web client calls it right after login with the
//	                       freshly-issued bearer token. It is how a client learns
//	                       its own tenant, and it must stay authenticated.
//	CheckPermission      — service-to-service, but pkg/iamclient dials iam with
//	                       ForwardAuthUnaryClientInterceptor, so the caller's own
//	                       bearer token already arrives. It additionally refuses a
//	                       user_id that is not the caller's, so it is no longer an
//	                       enumeration oracle (#139 §4).
//	ValidateToken        — retired to Unimplemented (#139 slice I). Services verify
//	                       JWTs in-process against the public key (#138); leaving it
//	                       public was an unauthenticated verification oracle.
var PublicMethods = map[string]string{
	iamv1.IAMService_Login_FullMethodName:            "caller has no token yet, by definition",
	iamv1.IAMService_RefreshToken_FullMethodName:     "presents a refresh token, not an access token",
	iamv1.IAMService_AcceptInvitation_FullMethodName: "invitee has no account yet; the invitation token is in the path",

	reflectionV1:      "reflection; only registered when Config.EnableReflection",
	reflectionV1Alpha: "reflection; only registered when Config.EnableReflection",
}
