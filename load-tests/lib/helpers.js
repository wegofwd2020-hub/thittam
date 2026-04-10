/**
 * Shared helpers for Thittam load tests.
 *
 * Environment variables (set in CI or pass with -e flag):
 *   BASE_URL    — Kong API Gateway root, e.g. http://localhost:8000
 *   TOKEN       — JWT access token for a pre-provisioned load-test user
 *   TENANT_ID   — UUID of the pre-provisioned load-test tenant
 *   PRODUCTION_ID — UUID of a pre-existing production (used by expense/budget tests)
 */

import http from "k6/http";
import { check } from "k6";

export const BASE_URL = __ENV.BASE_URL || "http://localhost:8000";
export const TOKEN = __ENV.TOKEN || "";
export const TENANT_ID = __ENV.TENANT_ID || "";
export const PRODUCTION_ID = __ENV.PRODUCTION_ID || "";

/** Standard JSON headers including tenant and auth. */
export function headers() {
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${TOKEN}`,
    "X-Tenant-ID": TENANT_ID,
  };
}

/**
 * Assert a response has an expected status and return whether the check passed.
 * Wraps k6 `check()` so failures appear in the summary with context.
 */
export function expectStatus(res, status, tag) {
  return check(res, {
    [`${tag}: status ${status}`]: (r) => r.status === status,
  });
}

/**
 * POST JSON and assert the response status.
 * Returns the parsed JSON body on success, null on failure.
 */
export function postJSON(path, body, expectedStatus = 201, tag = path) {
  const res = http.post(`${BASE_URL}${path}`, JSON.stringify(body), {
    headers: headers(),
    tags: { name: tag },
  });
  expectStatus(res, expectedStatus, tag);
  if (res.status !== expectedStatus) return null;
  try {
    return JSON.parse(res.body);
  } catch (_) {
    return null;
  }
}

/**
 * GET and assert the response status.
 * Returns the parsed JSON body on success, null on failure.
 */
export function getJSON(path, expectedStatus = 200, tag = path) {
  const res = http.get(`${BASE_URL}${path}`, {
    headers: headers(),
    tags: { name: tag },
  });
  expectStatus(res, expectedStatus, tag);
  if (res.status !== expectedStatus) return null;
  try {
    return JSON.parse(res.body);
  } catch (_) {
    return null;
  }
}

/** Generate a random amount between min and max (inclusive), as a string with 2dp. */
export function randomAmount(min = 1000, max = 100000) {
  const raw = min + Math.random() * (max - min);
  return raw.toFixed(2);
}

/** Random element from an array. */
export function pick(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}
