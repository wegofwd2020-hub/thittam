import { ApiError } from "@/lib/api/error";
import { lookupKeys, requestKey } from "./keys";
import fixtures from "./fixtures.generated.json";

export interface DemoMeta {
  capturedAt: string;
  tenant: string;
  demoEmail: string;
}

interface FixtureFile {
  _meta: DemoMeta;
  responses: Record<string, unknown>;
}

const file = fixtures as unknown as FixtureFile;

/** Writes that are really reads. Login is a POST but changes no state. */
const ALLOWED_WRITES = new Set(["POST /api/v1/auth/login"]);

function demoError(status: number, code: string, message: string): ApiError {
  return new ApiError(status, {
    code,
    message,
    details: undefined,
    request_id: "demo",
  });
}

export function demoMeta(): DemoMeta {
  return file._meta;
}

/**
 * Resolve a request against the recorded fixtures.
 *
 * Throws rather than returning a fallback: a demo that silently shows nothing
 * hides its own gaps, and the app already renders ApiError properly.
 */
export function demoRespond<T>(method: string, path: string): T {
  const upper = method.toUpperCase();

  if (upper !== "GET" && !ALLOWED_WRITES.has(requestKey(upper, path))) {
    throw demoError(
      501,
      "DEMO_READ_ONLY",
      "This is a read-only demo — changes cannot be saved.",
    );
  }

  for (const key of lookupKeys(upper, path)) {
    if (Object.prototype.hasOwnProperty.call(file.responses, key)) {
      // Deep copy: React Query hands this object to components that may sort
      // or splice in place, which would otherwise corrupt the next lookup.
      return structuredClone(file.responses[key]) as T;
    }
  }

  throw demoError(
    501,
    "DEMO_NOT_RECORDED",
    `This page is not part of the demo (${requestKey(upper, path)}).`,
  );
}
