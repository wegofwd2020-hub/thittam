import { describe, it, expect, vi, beforeEach } from "vitest";
import { ApiError } from "@/lib/api/error";

const fixtures = {
  _meta: {
    capturedAt: "2026-07-22T10:00:00Z",
    tenant: "xyz-cba",
    demoEmail: "rajesh.kumar@xyzcba.com",
  },
  responses: {
    "GET /api/v1/productions": { productions: [{ id: "p1" }] },
    "POST /api/v1/auth/login": { access_token: "demo", refresh_token: "demo" },
  },
};

vi.mock("./fixtures.generated.json", () => ({ default: fixtures }));

let demoRespond: typeof import("./transport").demoRespond;
let demoMeta: typeof import("./transport").demoMeta;

beforeEach(async () => {
  const mod = await import("./transport");
  demoRespond = mod.demoRespond;
  demoMeta = mod.demoMeta;
});

describe("demoRespond", () => {
  it("returns the recorded body for a known request", () => {
    expect(demoRespond("GET", "/api/v1/productions")).toEqual({
      productions: [{ id: "p1" }],
    });
  });

  it("falls back to the unfiltered recording when a query string misses", () => {
    expect(demoRespond("GET", "/api/v1/productions?status=active")).toEqual({
      productions: [{ id: "p1" }],
    });
  });

  it("returns a deep copy so callers cannot mutate the fixtures", () => {
    const first = demoRespond<{ productions: { id: string }[] }>(
      "GET",
      "/api/v1/productions",
    );
    first.productions[0].id = "mutated";
    const second = demoRespond<{ productions: { id: string }[] }>(
      "GET",
      "/api/v1/productions",
    );
    expect(second.productions[0].id).toBe("p1");
  });

  it("throws a 501 ApiError for an unrecorded read", () => {
    expect(() => demoRespond("GET", "/api/v1/expenses")).toThrowError(ApiError);
    try {
      demoRespond("GET", "/api/v1/expenses");
    } catch (err) {
      expect((err as ApiError).status).toBe(501);
      expect((err as ApiError).code).toBe("DEMO_NOT_RECORDED");
    }
  });

  it("throws a 501 DEMO_READ_ONLY for any write, even a recorded path", () => {
    try {
      demoRespond("PATCH", "/api/v1/productions");
      throw new Error("should have thrown");
    } catch (err) {
      expect((err as ApiError).code).toBe("DEMO_READ_ONLY");
    }
  });

  it("still serves the recorded login POST", () => {
    expect(demoRespond("POST", "/api/v1/auth/login")).toEqual({
      access_token: "demo",
      refresh_token: "demo",
    });
  });
});

describe("demoMeta", () => {
  it("exposes the captured metadata", () => {
    expect(demoMeta().demoEmail).toBe("rajesh.kumar@xyzcba.com");
  });
});
