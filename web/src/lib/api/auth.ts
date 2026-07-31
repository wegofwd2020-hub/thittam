import { env } from "@/env";
import { demoRespond } from "@/demo/transport";
import type { TokenPair } from "./types";
import { ApiError } from "./client";
import type { ApiErrorBody } from "./types";

async function authRequest<T>(
  path: string,
  body: unknown,
): Promise<T> {
  // This module does not go through ApiClient — it has its own fetch — so it
  // needs the demo branch of its own. Without it, login still hits the network
  // in a demo build and the whole thing stalls on the first request.
  if (env.demoMode) {
    return demoRespond<T>("POST", path);
  }

  const res = await fetch(`${env.platformApiUrl}${path}`, {
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
  return `${env.platformApiUrl}/api/v1/auth/sso/authorize?tenant=${encodeURIComponent(tenantSlug)}`;
}
