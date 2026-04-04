export const env = {
  /** Base URL for the Kong API gateway (or direct service in dev). */
  apiUrl:
    process.env.NEXT_PUBLIC_API_URL || "http://localhost:8086",

  /**
   * Base URL for platform-level APIs (auth, tenant provisioning).
   * Defaults to the IAM service in local dev.
   */
  platformApiUrl:
    process.env.NEXT_PUBLIC_PLATFORM_API_URL || "http://localhost:8086",
} as const;
