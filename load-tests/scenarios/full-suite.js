/**
 * Full load test suite — runs all four scenarios concurrently.
 *
 * Combines:
 *   1. expense_submissions  — 100 VUs, 4 minutes
 *   2. budget_approvals     — 50 VUs, 4 minutes
 *   3. ledger_throughput    — 17 req/s arrival rate, 10 minutes
 *   4. report_generation    — 10 VUs, 5 minutes (read-path resilience)
 *
 * The ledger_throughput scenario has the longest duration (10 minutes).
 * Other scenarios complete first and do not block it.
 *
 * Overall acceptance criteria (union of per-scenario thresholds):
 *   All per-scenario thresholds must pass.
 *   Additionally: total http_req_failed rate < 0.001 across all scenarios.
 *
 * After this test completes, run the ledger consistency verifier:
 *   scripts/verify-ledger-balance.sh
 */

import { sleep } from "k6";
import http from "k6/http";
import { check } from "k6";
import { BASE_URL, headers, PRODUCTION_ID, pick, randomAmount } from "../lib/helpers.js";

const DEBIT_ACCOUNT_ID = __ENV.DEBIT_ACCOUNT_ID || "00000000-0000-0000-0000-000000000001";
const CREDIT_ACCOUNT_ID = __ENV.CREDIT_ACCOUNT_ID || "00000000-0000-0000-0000-000000000002";
const PERIOD_ID = __ENV.PERIOD_ID || "00000000-0000-0000-0000-000000000010";
const EXPENSE_CATEGORIES = ["location_rental", "catering", "equipment_hire", "transport"];
const REPORT_TYPES = ["cost_report", "budget_utilisation", "expense_by_category"];

export const options = {
  scenarios: {
    expense_submissions: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 100 },
        { duration: "3m", target: 100 },
        { duration: "30s", target: 0 },
      ],
      exec: "runExpenseSubmission",
      gracefulRampDown: "10s",
    },
    budget_approvals: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "20s", target: 50 },
        { duration: "3m", target: 50 },
        { duration: "20s", target: 0 },
      ],
      exec: "runBudgetApproval",
      gracefulRampDown: "10s",
    },
    ledger_throughput: {
      executor: "constant-arrival-rate",
      rate: 17,
      timeUnit: "1s",
      duration: "10m",
      preAllocatedVUs: 40,
      maxVUs: 80,
      exec: "runLedgerEntry",
    },
    report_generation: {
      executor: "constant-vus",
      vus: 10,
      duration: "5m",
      exec: "runReportGeneration",
    },
  },
  thresholds: {
    "http_req_duration{name:/api/v1/expenses}": ["p(95)<500", "p(99)<1000"],
    "http_req_duration{name:submit-budget}": ["p(95)<400", "p(99)<800"],
    "http_req_duration{name:approve-budget}": ["p(95)<400", "p(99)<800"],
    "http_req_duration{name:create-journal-entry}": ["p(95)<300", "p(99)<600"],
    "http_req_duration{name:post-journal-entry}": ["p(95)<400", "p(99)<800"],
    "http_req_duration{name:generate-report}": ["p(95)<2000", "p(99)<5000"],
    http_req_failed: ["rate<0.001"],
    checks: ["rate>0.999"],
    http_reqs: ["count>9000"], // ledger throughput check
  },
};

// Pre-created budget IDs for budget_approvals scenario
let budgetIDs = [];

export function setup() {
  const ids = [];
  for (let i = 0; i < 50; i++) {
    const res = http.post(
      `${BASE_URL}/api/v1/budgets`,
      JSON.stringify({
        production_id: PRODUCTION_ID,
        label: `Suite Budget ${i}`,
        currency: "INR",
        total_amount: "500000.00",
      }),
      { headers: headers() },
    );
    if (res.status === 201) {
      try {
        ids.push(JSON.parse(res.body).id);
      } catch (_) {}
    }
  }
  return { budgetIDs: ids };
}

export function runExpenseSubmission() {
  http.post(
    `${BASE_URL}/api/v1/expenses`,
    JSON.stringify({
      production_id: PRODUCTION_ID,
      category_id: pick(EXPENSE_CATEGORIES),
      amount: randomAmount(500, 50000),
      currency: "INR",
      description: `Suite expense ${Date.now()}`,
      vendor_name: "Load Test Vendor",
    }),
    { headers: headers(), tags: { name: "/api/v1/expenses" } },
  );
  sleep(0.1 + Math.random() * 0.4);
}

export function runBudgetApproval(data) {
  const { budgetIDs } = data;
  if (!budgetIDs || budgetIDs.length === 0) return;
  const id = budgetIDs[__VU % budgetIDs.length];

  http.post(`${BASE_URL}/api/v1/budgets/${id}/submit`, null, {
    headers: headers(),
    tags: { name: "submit-budget" },
  });
  sleep(0.05);
  http.post(`${BASE_URL}/api/v1/budgets/${id}/approve`, null, {
    headers: headers(),
    tags: { name: "approve-budget" },
  });
  sleep(0.2 + Math.random() * 0.3);
}

export function runLedgerEntry() {
  const amount = randomAmount(100, 10000);
  const createRes = http.post(
    `${BASE_URL}/api/v1/ledger/entries`,
    JSON.stringify({
      production_id: PRODUCTION_ID,
      period_id: PERIOD_ID,
      narration: `Suite entry ${Date.now()}`,
      lines: [
        { account_id: DEBIT_ACCOUNT_ID, debit: amount, credit: "0.00" },
        { account_id: CREDIT_ACCOUNT_ID, debit: "0.00", credit: amount },
      ],
    }),
    { headers: headers(), tags: { name: "create-journal-entry" } },
  );
  if (createRes.status !== 201) return;
  let entry;
  try { entry = JSON.parse(createRes.body); } catch (_) { return; }
  sleep(0.01);
  http.post(
    `${BASE_URL}/api/v1/ledger/entries/${entry.id}/post`,
    null,
    { headers: headers(), tags: { name: "post-journal-entry" } },
  );
}

export function runReportGeneration() {
  const reportType = pick(REPORT_TYPES);
  const res = http.get(
    `${BASE_URL}/api/v1/reports/${reportType}?production_id=${PRODUCTION_ID}`,
    { headers: headers(), tags: { name: "generate-report" } },
  );
  check(res, { "report: 200": (r) => r.status === 200 });
  sleep(2 + Math.random() * 3);
}

// default export required by k6 even when using named exec functions
export default function () {}
