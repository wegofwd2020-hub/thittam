// Local-dev defaults point at the grpc-gateway HTTP port (9086 for IAM),
// not the bare gRPC port (8086) which speaks no HTTP. Once Kong (#60 Phase B)
// is in front of everything, set NEXT_PUBLIC_API_URL=http://localhost:8000
// to route through it instead.
export const env = {
  /** Base URL for the Kong API gateway (or direct grpc-gateway port in dev). */
  apiUrl:
    process.env.NEXT_PUBLIC_API_URL || "http://localhost:9086",

  /**
   * Base URL for platform-level APIs (auth, tenant provisioning).
   * Defaults to the IAM grpc-gateway port in local dev.
   */
  platformApiUrl:
    process.env.NEXT_PUBLIC_PLATFORM_API_URL || "http://localhost:9086",
} as const;
