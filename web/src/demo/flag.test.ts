import { describe, it, expect, afterEach } from "vitest";
import { isDemo } from "./flag";

const original = process.env.NEXT_PUBLIC_DEMO;

afterEach(() => {
  process.env.NEXT_PUBLIC_DEMO = original;
});

describe("isDemo", () => {
  it("is true when NEXT_PUBLIC_DEMO is exactly '1'", () => {
    process.env.NEXT_PUBLIC_DEMO = "1";
    expect(isDemo()).toBe(true);
  });

  it("is false when unset", () => {
    delete process.env.NEXT_PUBLIC_DEMO;
    expect(isDemo()).toBe(false);
  });

  it("is false for other truthy-looking values", () => {
    process.env.NEXT_PUBLIC_DEMO = "true";
    expect(isDemo()).toBe(false);
  });
});
