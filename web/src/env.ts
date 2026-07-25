// Local-dev routing for the grpc-gateway HTTP ports. Each service exposes its
// own gateway on a dedicated port (iam: 9086, project-management: 9080,
// budget-planning: 9081). Once Kong (#60 Phase B) is in front of everything,
// set NEXT_PUBLIC_API_URL=http://localhost:8500 and it will route to all.
//
// When a NEXT_PUBLIC_* env var is set, it wins. Otherwise, the URL is derived
// from window.location.hostname at call time so the app works from any host —
// localhost for single-machine dev, or a LAN IP / hostname for customer demos
// served over the network. SSR paths get "http://localhost:<port>" as a
// placeholder; API calls only fire client-side after hydration.

function hostUrl(port: number): string {
  if (typeof window !== "undefined" && window.location?.hostname) {
    return `${window.location.protocol}//${window.location.hostname}:${port}`;
  }
  return `http://localhost:${port}`;
}

export const env = {
  /** Single gateway URL (Kong). When unset, client routes per-service in dev. */
  get apiUrl(): string {
    return process.env.NEXT_PUBLIC_API_URL || "";
  },

  /** IAM grpc-gateway — default for auth, users, tenants. */
  get iamApiUrl(): string {
    return process.env.NEXT_PUBLIC_IAM_URL || hostUrl(9086);
  },

  /** project-management grpc-gateway — productions, phases, crew. */
  get projectApiUrl(): string {
    return process.env.NEXT_PUBLIC_PROJECT_URL || hostUrl(9080);
  },

  /** budget-planning grpc-gateway — budgets, line items, categories. */
  get budgetApiUrl(): string {
    return process.env.NEXT_PUBLIC_BUDGET_URL || hostUrl(9081);
  },

  /**
   * Base URL for platform-level APIs (auth, tenant provisioning).
   * Defaults to the IAM grpc-gateway port in local dev.
   */
  get platformApiUrl(): string {
    return process.env.NEXT_PUBLIC_PLATFORM_API_URL || hostUrl(9086);
  },
};
