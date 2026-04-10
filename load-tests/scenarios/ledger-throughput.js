/**
 * Load test: 1 000 ledger journal entries per minute, sustained for 10 minutes.
 *
 * Target throughput: 1 000 / 60 ≈ 16.7 req/s
 * Total entries:     ~10 000 over the run
 *
 * The double-entry constraint (debit total = credit total) is enforced by the
 * service. Any entry that returns 422 (validation error) is a test failure.
 * After the run, the ledger consistency check (see verify-ledger-balance.js)
 * queries the database to confirm all entries balance.
 *
 * Acceptance criteria:
 *   Throughput     ≥ 900 entries/min (90% of target; allows for think time jitter)
 *   p95 latency    < 300 ms
 *   p99 latency    < 600 ms
 *   Error rate     < 0.01% (financial operations have stricter SLA)
 *   422 rate       = 0%    (no malformed entries from the test harness)
 */

import { sleep, check } from "k6";
import http from "k6/http";
import { BASE_URL, headers, PRODUCTION_ID, randomAmount } from "../lib/helpers.js";

// Pre-seeded account IDs. Set via env vars for the target environment.
const DEBIT_ACCOUNT_ID = __ENV.DEBIT_ACCOUNT_ID || "00000000-0000-0000-0000-000000000001";
const CREDIT_ACCOUNT_ID = __ENV.CREDIT_ACCOUNT_ID || "00000000-0000-0000-0000-000000000002";
const PERIOD_ID = __ENV.PERIOD_ID || "00000000-0000-0000-0000-000000000010";

export const options = {
  scenarios: {
    ledger_throughput: {
      executor: "constant-arrival-rate",
      rate: 17,           // ~1 000/min; arrival-rate executor maintains this regardless of VU count
      timeUnit: "1s",
      duration: "10m",
      preAllocatedVUs: 40,  // enough to absorb ~300ms p99 latency
      maxVUs: 80,
    },
  },
  thresholds: {
    "http_req_duration{name:create-journal-entry}": ["p(95)<300", "p(99)<600"],
    "http_req_duration{name:post-journal-entry}": ["p(95)<400", "p(99)<800"],
    "http_req_failed": ["rate<0.0001"],   // < 0.01% — stricter than other tests
    checks: ["rate>0.9999"],
    // Throughput: at 17 req/s × 600s = 10 200 total. Require ≥9 000 to succeed.
    http_reqs: ["count>9000"],
  },
};

export default function () {
  const amount = randomAmount(100, 10000);

  // Step 1: Create a draft journal entry (double-entry: debit + credit must balance)
  const createRes = http.post(
    `${BASE_URL}/api/v1/ledger/entries`,
    JSON.stringify({
      production_id: PRODUCTION_ID,
      period_id: PERIOD_ID,
      narration: `Load test entry ${Date.now()}`,
      lines: [
        { account_id: DEBIT_ACCOUNT_ID, debit: amount, credit: "0.00" },
        { account_id: CREDIT_ACCOUNT_ID, debit: "0.00", credit: amount },
      ],
    }),
    { headers: headers(), tags: { name: "create-journal-entry" } },
  );

  const created = check(createRes, {
    "create: 201": (r) => r.status === 201,
    "create: not 422": (r) => r.status !== 422, // 422 = validation error (test harness bug)
  });

  if (!created) return;

  let entry;
  try {
    entry = JSON.parse(createRes.body);
  } catch (_) {
    return;
  }

  sleep(0.01); // minimal pause before posting

  // Step 2: Post the entry (moves it from draft → posted)
  const postRes = http.post(
    `${BASE_URL}/api/v1/ledger/entries/${entry.id}/post`,
    null,
    { headers: headers(), tags: { name: "post-journal-entry" } },
  );

  check(postRes, {
    "post: 200": (r) => r.status === 200,
  });
}
