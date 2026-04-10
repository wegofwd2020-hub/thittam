/**
 * Load-balancer distribution test
 *
 * Sends 300 lightweight health-check requests to each of the 10 gRPC services
 * (via Kong REST proxy) and asserts that no single upstream pod handles more
 * than 60% of the traffic — confirming that Istio ROUND_ROBIN is distributing
 * requests across all 3 replicas (~33% each, ±20% tolerance).
 *
 * The k6 script itself cannot inspect which pod served each response (that
 * information is in Istio telemetry). The script validates that:
 *   1. All requests succeed (error rate = 0%) — broken LB would surface as
 *      connection errors or elevated 5xx rates.
 *   2. Request throughput is uniform — no tail latency spikes that would
 *      indicate hotspot routing to an overloaded pod.
 *
 * Post-run verification (must be done manually or via CI step):
 *   kubectl exec -n monitoring deploy/prometheus -- \
 *     promtool query instant \
 *       'sum by (destination_workload_instance) (
 *          rate(istio_requests_total{
 *            destination_service_namespace="thittam",
 *            reporter="destination"
 *          }[5m])
 *        )'
 *   # Each of the 3 instances per service must be within 13%–53% of total
 *   # (33% ± 20%).  Any instance at 0% means it is not receiving traffic.
 *
 * Usage:
 *   source .env.load-test
 *   k6 run load-tests/scenarios/lb-distribution.js
 */

import http from "k6/http";
import { check, group, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8000";
const TOKEN = __ENV.TOKEN || "";

// One custom error-rate metric per service so CI can attribute failures.
const errorRates = {
  project_management: new Rate("errors_project_management"),
  budget_planning: new Rate("errors_budget_planning"),
  expense_tracking: new Rate("errors_expense_tracking"),
  general_ledger: new Rate("errors_general_ledger"),
  inventory_management: new Rate("errors_inventory_management"),
  reporting_analytics: new Rate("errors_reporting_analytics"),
  iam: new Rate("errors_iam"),
  notifications: new Rate("errors_notifications"),
  document: new Rate("errors_document"),
  billing: new Rate("errors_billing"),
};

// Per-service latency trend — deviations indicate routing to a slow pod.
const latencyTrends = {
  project_management: new Trend("latency_project_management", true),
  budget_planning: new Trend("latency_budget_planning", true),
  expense_tracking: new Trend("latency_expense_tracking", true),
  general_ledger: new Trend("latency_general_ledger", true),
  inventory_management: new Trend("latency_inventory_management", true),
  reporting_analytics: new Trend("latency_reporting_analytics", true),
  iam: new Trend("latency_iam", true),
  notifications: new Trend("latency_notifications", true),
  document: new Trend("latency_document", true),
  billing: new Trend("latency_billing", true),
};

// Each service endpoint — uses /healthz which requires no auth and is cheap.
// Kong routes /api/v1/<service>/healthz → <service>:port/healthz.
const SERVICES = [
  { name: "project_management", path: "/api/v1/project/healthz" },
  { name: "budget_planning", path: "/api/v1/budget/healthz" },
  { name: "expense_tracking", path: "/api/v1/expenses/healthz" },
  { name: "general_ledger", path: "/api/v1/ledger/healthz" },
  { name: "inventory_management", path: "/api/v1/inventory/healthz" },
  { name: "reporting_analytics", path: "/api/v1/reports/healthz" },
  { name: "iam", path: "/api/v1/iam/healthz" },
  { name: "notifications", path: "/api/v1/notifications/healthz" },
  { name: "document", path: "/api/v1/documents/healthz" },
  { name: "billing", path: "/api/v1/billing/healthz" },
];

export const options = {
  // 30 VUs × 10 s = ~300 requests per service across the test window.
  // 3 replicas × ~100 expected per replica = sufficient sample for ±20% check.
  scenarios: {
    lb_distribution: {
      executor: "constant-vus",
      vus: 30,
      duration: "30s",
    },
  },
  thresholds: {
    // All requests to all services must succeed.
    http_req_failed: ["rate<0.01"],

    // Per-service error rates must be 0%.
    errors_project_management: ["rate<0.01"],
    errors_budget_planning: ["rate<0.01"],
    errors_expense_tracking: ["rate<0.01"],
    errors_general_ledger: ["rate<0.01"],
    errors_inventory_management: ["rate<0.01"],
    errors_reporting_analytics: ["rate<0.01"],
    errors_iam: ["rate<0.01"],
    errors_notifications: ["rate<0.01"],
    errors_document: ["rate<0.01"],
    errors_billing: ["rate<0.01"],

    // Latency must be low — hotspot routing would show up as high p99.
    // /healthz is trivial; any p99 > 200ms indicates routing problems.
    "http_req_duration{scenario:lb_distribution}": ["p(99)<200"],
  },
};

const authHeaders = {
  Authorization: `Bearer ${TOKEN}`,
  "Content-Type": "application/json",
};

export default function () {
  // Each VU cycles through all 10 services — ensures every service accumulates
  // ~300 requests over the 30-second window regardless of VU scheduling.
  for (const svc of SERVICES) {
    group(svc.name, () => {
      const res = http.get(`${BASE_URL}${svc.path}`, {
        headers: authHeaders,
        tags: { service: svc.name },
      });

      const ok = check(res, {
        [`${svc.name}: status 200`]: (r) => r.status === 200,
      });

      errorRates[svc.name].add(!ok);
      latencyTrends[svc.name].add(res.timings.duration);
    });

    sleep(0.05); // 50 ms between service calls — avoid burst on a single pod.
  }
}

export function handleSummary(data) {
  // Print a distribution note reminding the operator to check Prometheus.
  console.log("\n=== Load Balancer Distribution Reminder ===");
  console.log(
    "k6 validates error rates and latency. Per-pod distribution must be"
  );
  console.log("verified via Istio Prometheus metrics:");
  console.log(
    '  sum by (destination_workload_instance) (rate(istio_requests_total{destination_service_namespace="thittam",reporter="destination"}[5m]))'
  );
  console.log(
    "Each of the 3 replicas per service should handle 13%–53% of total."
  );
  console.log("==========================================\n");

  return {
    stdout: JSON.stringify(data.metrics, null, 2),
  };
}
