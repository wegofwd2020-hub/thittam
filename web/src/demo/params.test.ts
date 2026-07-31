import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("./fixtures.generated.json", () => ({
  default: {
    _meta: { capturedAt: "", tenant: "xyz-cba", demoEmail: "a@b.c" },
    responses: {
      "GET /api/v1/productions": {},
      "GET /api/v1/productions/p1": {},
      "GET /api/v1/productions/p2": {},
      "GET /api/v1/productions/p1/phases": {},
      "GET /api/v1/budgets/b1": {},
    },
  },
}));

let idsForCollection: typeof import("./params").idsForCollection;

beforeEach(async () => {
  idsForCollection = (await import("./params")).idsForCollection;
});

describe("idsForCollection", () => {
  it("finds every recorded detail id", () => {
    expect(idsForCollection("productions").sort()).toEqual(["p1", "p2"]);
  });

  it("ignores nested sub-resources", () => {
    expect(idsForCollection("productions")).not.toContain("phases");
  });

  it("ignores the bare list key", () => {
    expect(idsForCollection("productions")).not.toContain("");
  });

  it("works for a different collection", () => {
    expect(idsForCollection("budgets")).toEqual(["b1"]);
  });

  it("returns an empty array for an unrecorded collection", () => {
    expect(idsForCollection("expenses")).toEqual([]);
  });
});
