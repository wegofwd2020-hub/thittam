/**
 * Load test: 100 concurrent expense submissions against a single production.
 *
 * Scenario:  100 VUs ramp up over 30s, sustain for 3 minutes, ramp down 30s.
 * Target:    ≥1 500 expense submissions total (≈8 req/s at peak).
 *
 * Acceptance criteria:
 *   p95 latency  < 500 ms
 *   p99 latency  < 1 000 ms
 *   Error rate   < 0.1%  (status !== 201 counts as an error)
 */

import { sleep } from "k6";
import { postJSON, PRODUCTION_ID, pick, randomAmount } from "../lib/helpers.js";

export const options = {
  scenarios: {
    expense_submissions: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 100 }, // ramp up
        { duration: "3m", target: 100 },  // sustain
        { duration: "30s", target: 0 },   // ramp down
      ],
      gracefulRampDown: "10s",
    },
  },
  thresholds: {
    "http_req_duration{name:/api/v1/expenses}": [
      "p(95)<500",
      "p(99)<1000",
    ],
    "http_req_failed{name:/api/v1/expenses}": ["rate<0.001"],
    checks: ["rate>0.999"], // ≥99.9% of checks must pass
  },
};

const CATEGORIES = ["location_rental", "catering", "equipment_hire", "transport"];

export default function () {
  postJSON(
    "/api/v1/expenses",
    {
      production_id: PRODUCTION_ID,
      category_id: pick(CATEGORIES),
      amount: randomAmount(500, 50000),
      currency: "INR",
      description: `Load test expense ${Date.now()}`,
      vendor_name: "Load Test Vendor",
    },
    201,
    "/api/v1/expenses",
  );

  // Realistic think time: 100–500ms between requests
  sleep(0.1 + Math.random() * 0.4);
}
