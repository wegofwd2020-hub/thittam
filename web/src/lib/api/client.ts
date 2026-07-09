import { env } from "@/env";
import type { ApiErrorBody } from "./types";

// ---------------------------------------------------------------------------
// Typed error
// ---------------------------------------------------------------------------
export class ApiError extends Error {
  public readonly code: string;
  public readonly details: Record<string, unknown> | undefined;
  public readonly requestId: string;
  public readonly status: number;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message);
    this.name = "ApiError";
    this.code = body.code;
    this.details = body.details;
    this.requestId = body.request_id;
    this.status = status;
  }
}

// ---------------------------------------------------------------------------
// Per-service base URL resolver
// ---------------------------------------------------------------------------
// In local dev each service owns its own grpc-gateway port (iam:9086,
// project-management:9080, budget-planning:9081). Kong (#60 Phase B) will
// eventually front everything on a single origin — when NEXT_PUBLIC_API_URL
// is set, it wins and the prefix table is bypassed.
function resolveBaseUrl(path: string): string {
  if (env.apiUrl) return env.apiUrl;

  if (
    path.startsWith("/api/v1/productions") ||
    path.startsWith("/api/v1/phases") ||
    path.startsWith("/api/v1/crew") ||
    path.startsWith("/api/v1/config/entity-labels") ||
    path.startsWith("/api/v1/config/phase-types")
  ) {
    return env.projectApiUrl;
  }

  if (
    path.startsWith("/api/v1/budgets") ||
    path.startsWith("/api/v1/line-items") ||
    path.startsWith("/api/v1/config/budget-categories") ||
    path.startsWith("/api/v1/config/budget-templates")
  ) {
    return env.budgetApiUrl;
  }

  return env.iamApiUrl;
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------
interface Caller {
  id: string;
  email: string;
  role: string;
}

class ApiClient {
  private token: string | null = null;
  private tenantId: string | null = null;
  private caller: Caller | null = null;

  // -- Auth helpers ---------------------------------------------------------

  setToken(token: string | null): void {
    this.token = token;
  }

  // tenantId is no longer sent as a header (#138) — the field survives only
  // because web/src/lib/auth/context.tsx still calls setTenantId(me.tenant.id)
  // and other UI code reads it for display/routing.
  setTenantId(tenantId: string | null): void {
    this.tenantId = tenantId;
  }

  // caller is no longer sent as a header (#138) — the field is unread now,
  // but setCaller is still called from web/src/lib/auth/context.tsx, so it
  // stays rather than breaking those call sites.
  setCaller(caller: Caller | null): void {
    this.caller = caller;
  }

  // -- HTTP verbs -----------------------------------------------------------
  //
  // grpc-gateway (UseProtoNames=true, EmitUnpopulated=true) serialises proto
  // messages flat at the response root — e.g. GetBudget returns the Budget
  // fields directly, not {data: Budget}. List RPCs return {<plural_name>: [...]}
  // (e.g. {productions: [...]}). Callers type T as the literal JSON shape.

  async get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  async getList<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  async post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  async patch<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("PATCH", path, body);
  }

  async delete(path: string): Promise<void> {
    await this.request<void>("DELETE", path);
  }

  // -- Internal request -----------------------------------------------------

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "application/json",
    };

    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }

    // Identity travels in the bearer token alone. The server derives the caller
    // and the tenant from its verified claims (#138) and ignores any
    // X-Caller-* / X-Tenant-Id header, so sending them is at best misleading.

    const url = `${resolveBaseUrl(path)}${path}`;

    const init: RequestInit = { method, headers };
    if (body !== undefined) {
      init.body = JSON.stringify(body);
    }

    const res = await fetch(url, init);

    // 204 No Content — nothing to parse
    if (res.status === 204) {
      return undefined as T;
    }

    const json: unknown = await res.json();

    if (!res.ok) {
      const errorBody = json as ApiErrorBody;
      throw new ApiError(res.status, {
        code: errorBody.code ?? "UNKNOWN_ERROR",
        message: errorBody.message ?? res.statusText,
        details: errorBody.details,
        request_id: errorBody.request_id ?? "",
      });
    }

    return json as T;
  }
}

// ---------------------------------------------------------------------------
// Singleton
// ---------------------------------------------------------------------------
export const api = new ApiClient();
