/**
 * Load test: report generation while expense + ledger loads are active.
 *
 * This test is designed to run concurrently with expense-submissions.js and
 * ledger-throughput.js (see full-suite.js). It verifies that the read-only
 * reporting-analytics service remains responsive under write pressure.
 *
 * Scenario:  10 VUs continuously requesting reports for 5 minutes.
 *            Lower concurrency than write tests — reports are expensive reads.
 *
 * Acceptance criteria:
 *   p95 latency  < 2 000 ms  (reports are heavier than CRUD operations)
 *   p99 latency  < 5 000 ms
 *   Error rate   < 0.1%
 *   No reports return stale data older than 30 seconds
 *     (checked via X-Data-As-Of response header when present)
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { BASE_URL, headers, PRODUCTION_ID, pick } from "../lib/helpers.js";

const REPORT_TYPES = ["cost_report", "budget_utilisation", "expense_by_category"];

export const options = {
  scenarios: {
    report_generation: {
      executor: "constant-vus",
      vus: 10,
      duration: "5m",
    },
  },
  thresholds: {
    "http_req_duration{name:generate-report}": ["p(95)<2000", "p(99)<5000"],
    "http_req_failed{name:generate-report}": ["rate<0.001"],
    checks: ["rate>0.999"],
  },
};

export default function () {
  const reportType = pick(REPORT_TYPES);

  const res = http.get(
    `${BASE_URL}/api/v1/reports/${reportType}?production_id=${PRODUCTION_ID}`,
    { headers: headers(), tags: { name: "generate-report" } },
  );

  check(res, {
    "report: 200": (r) => r.status === 200,
    "report: has data": (r) => {
      try {
        const body = JSON.parse(r.body);
        return body && (body.data !== undefined || body.rows !== undefined);
      } catch (_) {
        return false;
      }
    },
    "report: data freshness ≤30s": (r) => {
      const asOf = r.headers["X-Data-As-Of"];
      if (!asOf) return true; // header absent = not enforced for this endpoint
      const staleness = (Date.now() - new Date(asOf).getTime()) / 1000;
      return staleness <= 30;
    },
  });

  // Reports are expensive — realistic delay between requests
  sleep(2 + Math.random() * 3);
}
