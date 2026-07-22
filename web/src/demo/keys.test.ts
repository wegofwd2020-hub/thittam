import { describe, it, expect } from "vitest";
import { requestKey, lookupKeys } from "./keys";

describe("requestKey", () => {
  it("joins an upper-cased method and the path", () => {
    expect(requestKey("get", "/api/v1/productions")).toBe(
      "GET /api/v1/productions",
    );
  });

  it("keeps the query string", () => {
    expect(requestKey("GET", "/api/v1/budgets?status=draft")).toBe(
      "GET /api/v1/budgets?status=draft",
    );
  });
});

describe("lookupKeys", () => {
  it("returns one key when there is no query string", () => {
    expect(lookupKeys("GET", "/api/v1/productions")).toEqual([
      "GET /api/v1/productions",
    ]);
  });

  it("returns the exact key first, then the query-stripped key", () => {
    expect(lookupKeys("GET", "/api/v1/budgets?status=draft")).toEqual([
      "GET /api/v1/budgets?status=draft",
      "GET /api/v1/budgets",
    ]);
  });

  it("does not emit a duplicate when the query string is empty", () => {
    expect(lookupKeys("GET", "/api/v1/budgets?")).toEqual([
      "GET /api/v1/budgets",
    ]);
  });
});
