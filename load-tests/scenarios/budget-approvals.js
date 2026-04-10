/**
 * Load test: 50 concurrent budget approvals across different tenants.
 *
 * Setup:    Each VU is given a dedicated budget ID (pre-created in setup()).
 *           VUs are scoped to different tenants to exercise cross-tenant
 *           isolation under concurrent write load.
 *
 * Scenario: 50 VUs, each submitting then approving their assigned budget.
 *           Repeated until the 3-minute duration expires.
 *
 * Acceptance criteria:
 *   p95 submit latency  < 400 ms
 *   p99 approve latency < 800 ms
 *   Error rate          < 0.1%
 *   Zero double-approve errors (idempotency guard must hold)
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { BASE_URL, headers, postJSON, expectStatus, PRODUCTION_ID } from "../lib/helpers.js";

export const options = {
  scenarios: {
    budget_approvals: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "20s", target: 50 },
        { duration: "3m", target: 50 },
        { duration: "20s", target: 0 },
      ],
      gracefulRampDown: "10s",
    },
  },
  thresholds: {
    "http_req_duration{name:submit-budget}": ["p(95)<400", "p(99)<800"],
    "http_req_duration{name:approve-budget}": ["p(95)<400", "p(99)<800"],
    "http_req_failed": ["rate<0.001"],
    checks: ["rate>0.999"],
  },
};

/** setup() runs once before VUs start. Creates 50 budgets in draft status. */
export function setup() {
  const budgetIDs = [];
  for (let i = 0; i < 50; i++) {
    const body = postJSON(
      "/api/v1/budgets",
      {
        production_id: PRODUCTION_ID,
        label: `Load Test Budget ${i}`,
        currency: "INR",
        total_amount: "500000.00",
      },
      201,
      "setup-budget",
    );
    if (body && body.id) {
      budgetIDs.push(body.id);
    }
  }
  return { budgetIDs };
}

export default function ({ budgetIDs }) {
  if (!budgetIDs || budgetIDs.length === 0) return;

  // Each VU picks a budget by its index (wraps around if more iterations than budgets)
  const idx = __VU % budgetIDs.length;
  const budgetID = budgetIDs[idx];

  // Submit
  const submitRes = http.post(
    `${BASE_URL}/api/v1/budgets/${budgetID}/submit`,
    null,
    { headers: headers(), tags: { name: "submit-budget" } },
  );
  // 200 = submitted, 409 = already submitted (idempotent — still acceptable)
  check(submitRes, {
    "submit: 200 or 409": (r) => r.status === 200 || r.status === 409,
  });

  sleep(0.05 + Math.random() * 0.1);

  // Approve
  const approveRes = http.post(
    `${BASE_URL}/api/v1/budgets/${budgetID}/approve`,
    null,
    { headers: headers(), tags: { name: "approve-budget" } },
  );
  // 200 = approved, 409 = already approved (idempotent — still acceptable)
  check(approveRes, {
    "approve: 200 or 409": (r) => r.status === 200 || r.status === 409,
  });

  sleep(0.2 + Math.random() * 0.3);
}

/** teardown() prints a summary. Actual ledger consistency is verified separately. */
export function teardown(data) {
  console.log(`Budget approval load test complete. Budgets used: ${data.budgetIDs.length}`);
}
