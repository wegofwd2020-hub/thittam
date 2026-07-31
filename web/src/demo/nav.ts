/**
 * Where the demo lands after login.
 *
 * NOT "/" — the dashboard is driven by six reporting-analytics endpoints
 * (src/lib/api/dashboard.ts) and that service exposes no grpc-gateway, so
 * nothing could be recorded for it.
 */
export const DEMO_LANDING = "/productions";

/** The only routes reachable in a demo build. */
const DEMO_ROUTES = new Set(["/login", "/productions", "/budgets"]);

export function isRouteInDemo(href: string): boolean {
  return DEMO_ROUTES.has(href);
}
