import { env } from "@/env";
import type { TokenPair } from "./types";
import { ApiError } from "./client";
import type { ApiErrorBody } from "./types";

const AUTH_BASE = env.platformApiUrl;

async function authRequest<T>(
  path: string,
  body: unknown,
): Promise<T> {
  const res = await fetch(`${AUTH_BASE}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const json: unknown = await res.json();

  if (!res.ok) {
    const errorBody = json as ApiErrorBody;
    throw new ApiError(res.status, {
      code: errorBody.code ?? "AUTH_ERROR",
      message: errorBody.message ?? res.statusText,
      details: errorBody.details,
      request_id: errorBody.request_id ?? "",
    });
  }

  return json as T;
}

/**
 * Authenticate with email and password credentials.
 *
 * The grpc-gateway returns the TokenPair flat at the top level — no
 * `{data: ...}` envelope. See thittam #60 Phase A.
 */
export async function login(
  email: string,
  password: string,
): Promise<TokenPair> {
  return authRequest<TokenPair>("/api/v1/auth/login", { email, password });
}

/**
 * Exchange a refresh token for a new token pair.
 */
export async function refreshToken(token: string): Promise<TokenPair> {
  return authRequest<TokenPair>("/api/v1/auth/refresh", {
    refresh_token: token,
  });
}

/**
 * Invalidate the given refresh token (server-side logout).
 */
export async function logout(token: string): Promise<void> {
  await authRequest<void>("/api/v1/auth/logout", {
    refresh_token: token,
  });
}

/**
 * Returns the SSO authorization URL for the given tenant slug.
 * The caller should redirect the browser to this URL.
 */
export function getSSOAuthorizeUrl(tenantSlug: string): string {
  return `${AUTH_BASE}/api/v1/auth/sso/authorize?tenant=${encodeURIComponent(tenantSlug)}`;
}
