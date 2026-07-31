import { describe, it, expect } from "vitest";
import { DEMO_LANDING, isRouteInDemo } from "./nav";

describe("DEMO_LANDING", () => {
  it("is /productions, not the dashboard", () => {
    // The dashboard calls reporting-analytics, which exposes no grpc-gateway.
    expect(DEMO_LANDING).toBe("/productions");
  });
});

describe("isRouteInDemo", () => {
  it("allows the slice", () => {
    expect(isRouteInDemo("/productions")).toBe(true);
    expect(isRouteInDemo("/budgets")).toBe(true);
  });

  it("rejects pages whose services have no REST surface", () => {
    expect(isRouteInDemo("/")).toBe(false);
    expect(isRouteInDemo("/expenses")).toBe(false);
    expect(isRouteInDemo("/inventory")).toBe(false);
    expect(isRouteInDemo("/reports")).toBe(false);
    expect(isRouteInDemo("/billing")).toBe(false);
  });

  it("rejects /projects, which is a dead link in the app itself", () => {
    expect(isRouteInDemo("/projects")).toBe(false);
  });
});
