// Local-dev routing for the grpc-gateway HTTP ports. Each service exposes its
// own gateway on a dedicated port (iam: 9086, project-management: 9080,
// budget-planning: 9081). Once Kong (#60 Phase B) is in front of everything,
// set NEXT_PUBLIC_API_URL=http://localhost:8000 and it will route to all.
//
// In local dev, the ApiClient uses `resolveApiUrl(path)` to pick the correct
// per-service base URL from the path prefix. When NEXT_PUBLIC_API_URL is set
// (production/Kong), every request goes through that single origin instead.
export const env = {
  /** Single gateway URL (Kong). When unset, client routes per-service in dev. */
  apiUrl: process.env.NEXT_PUBLIC_API_URL || "",

  /** IAM grpc-gateway — default for auth, users, tenants. */
  iamApiUrl:
    process.env.NEXT_PUBLIC_IAM_URL || "http://localhost:9086",

  /** project-management grpc-gateway — productions, phases, crew. */
  projectApiUrl:
    process.env.NEXT_PUBLIC_PROJECT_URL || "http://localhost:9080",

  /** budget-planning grpc-gateway — budgets, line items, categories. */
  budgetApiUrl:
    process.env.NEXT_PUBLIC_BUDGET_URL || "http://localhost:9081",

  /**
   * Base URL for platform-level APIs (auth, tenant provisioning).
   * Defaults to the IAM grpc-gateway port in local dev.
   */
  platformApiUrl:
    process.env.NEXT_PUBLIC_PLATFORM_API_URL || "http://localhost:9086",
} as const;
