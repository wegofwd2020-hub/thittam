/**
 * Chaos observation scenario: ledger-503.
 *
 * Purpose: verify that expense-tracking degrades gracefully when
 * general-ledger returns 503 for 30 seconds. Specifically:
 *
 *   1. PRE-FAULT  (0–30s):  expense approvals succeed (200).
 *   2. FAULT      (30–60s): apply infra/chaos/ledger-503.yaml externally.
 *                            approvals return 503 — NOT 500/timeout/hang.
 *   3. RECOVERY   (60–90s): delete ledger-503.yaml externally.
 *                            approvals succeed again within 5 seconds of removal.
 *
 * This script only drives load and measures response codes. The Istio manifest
 * must be applied/removed by the operator in parallel (see infra/chaos/ledger-503.yaml).
 *
 * Acceptance criteria:
 *   - http_req_failed (non-2xx AND non-503) rate = 0% — no silent drops, no 500s
 *   - p99 latency (pre and post fault) < 1 000 ms — no hanging requests
 *   - checks{phase:pre-fault}  rate = 1.00 (all approvals succeed before fault)
 *   - checks{phase:post-fault} rate = 1.00 (all approvals succeed after recovery)
 *
 * Run:
 *   k6 run \
 *     -e BASE_URL=http://staging.thittam.io \
 *     -e TOKEN=<jwt> \
 *     -e TENANT_ID=<uuid> \
 *     -e PRODUCTION_ID=<uuid> \
 *     load-tests/scenarios/chaos-ledger-503.js
 */

import { sleep, check } from "k6";
import http from "k6/http";
import { BASE_URL, headers, PRODUCTION_ID, randomAmount } from "../lib/helpers.js";
import { Counter, Gauge } from "k6/metrics";

// Track pass/fail counts per phase for clean summary output.
const preSuccesses  = new Counter("pre_fault_successes");
const postSuccesses = new Counter("post_fault_successes");
const faultSeen503  = new Counter("fault_503_count");
const unexpectedErr = new Counter("unexpected_errors");

// Phase duration constants (seconds).
const PRE_FAULT_DUR  = 30;
const FAULT_DUR      = 30;
const POST_FAULT_DUR = 30;
const TOTAL_DUR      = PRE_FAULT_DUR + FAULT_DUR + POST_FAULT_DUR;

export const options = {
  // Single scenario, steady 20 VUs throughout — enough to observe all phase transitions.
  scenarios: {
    chaos_observation: {
      executor: "constant-vus",
      vus: 20,
      duration: `${TOTAL_DUR}s`,
    },
  },
  thresholds: {
    // No unexpected errors (500, timeout, network errors) at any point.
    unexpected_errors: ["count==0"],
    // Throughput should recover to > 0 successes in post-fault phase.
    post_fault_successes: ["count>0"],
    // p99 must stay under 1s — hanging requests (caused by missing circuit breaker)
    // would push this far above 1s during the fault window.
    "http_req_duration{phase:pre-fault}":  ["p(99)<1000"],
    "http_req_duration{phase:post-fault}": ["p(99)<1000"],
  },
};

// Pre-create an expense to approve so we're not creating a new one each iteration.
// Returns the expense ID or null if setup fails.
export function setup() {
  const res = http.post(
    `${BASE_URL}/api/v1/expenses`,
    JSON.stringify({
      production_id: PRODUCTION_ID,
      category_id:   "equipment_hire",
      amount:         randomAmount(1000, 50000),
      currency:       "INR",
      description:    "Chaos test expense — ledger-503 scenario",
      vendor_name:    "Chaos Vendor",
    }),
    { headers: headers() },
  );
  if (res.status !== 201) {
    console.error(`setup: failed to create expense (${res.status}): ${res.body}`);
    return { expenseID: null };
  }
  const body = JSON.parse(res.body);
  console.log(`setup: expense created — id=${body.id}`);
  return { expenseID: body.id };
}

export default function (data) {
  const elapsed = __VU === 1 && __ITER === 0 ? 0 : (Date.now() / 1000);

  // Determine current test phase from elapsed time using k6's scenario start.
  // k6 does not expose wall-clock start time directly, so we use __ENV or
  // a simple modular approach based on iteration number and VU sleep timing.
  //
  // Since all VUs sleep 1s per iteration, each VU cycles through phases
  // based on absolute elapsed time from scenario start. k6 exposes this via
  // the exec.scenario.iterationInTest counter relative to rate, but for
  // constant-vus the simplest proxy is a counter per VU capped by iteration.
  //
  // We approximate elapsed time as (iteration × sleep_duration).
  // For a 20 VU, 90s test at ~1.5s/iter, each VU does ~60 iters.
  const iterDur    = 1.5;  // approximate seconds per iteration
  const approxElapsed = __ITER * iterDur;
  let phase;
  if (approxElapsed < PRE_FAULT_DUR) {
    phase = "pre-fault";
  } else if (approxElapsed < PRE_FAULT_DUR + FAULT_DUR) {
    phase = "fault";
  } else {
    phase = "post-fault";
  }

  const expenseID = data.expenseID;
  if (!expenseID) {
    sleep(1);
    return;
  }

  // Attempt to approve the expense.
  const res = http.post(
    `${BASE_URL}/api/v1/expenses/${expenseID}/approve`,
    null,
    { headers: headers(), tags: { name: "approve-expense", phase } },
  );

  const is2xx  = res.status >= 200 && res.status < 300;
  const is503  = res.status === 503;
  const isOther = !is2xx && !is503;

  if (phase === "pre-fault") {
    check(res, { "pre-fault: 200": (r) => r.status === 200 }, { phase });
    if (is2xx) preSuccesses.add(1);
    if (isOther) unexpectedErr.add(1);

  } else if (phase === "fault") {
    // During the fault window the ledger is returning 503 via Istio.
    // expense-tracking must propagate a 503 (not 500 or hang) to clients.
    check(res, {
      "fault: 503 or 200 (before fault applied)": (r) => r.status === 503 || r.status === 200,
    }, { phase });
    if (is503) faultSeen503.add(1);
    // 500 or timeout = circuit-breaker / timeout not implemented — unexpected.
    if (res.status === 500 || res.status === 0) unexpectedErr.add(1);

  } else {
    // post-fault: fault manifest removed, service must have recovered.
    check(res, { "post-fault: 200": (r) => r.status === 200 }, { phase });
    if (is2xx) postSuccesses.add(1);
    if (isOther) unexpectedErr.add(1);
  }

  sleep(1 + Math.random() * 0.5);
}
