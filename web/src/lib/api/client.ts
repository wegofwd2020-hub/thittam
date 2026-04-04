import { env } from "@/env";
import type { ApiErrorBody, ApiListResponse, ApiResponse } from "./types";

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
// Client
// ---------------------------------------------------------------------------
class ApiClient {
  private baseUrl: string;
  private token: string | null = null;
  private tenantId: string | null = null;

  constructor(baseUrl?: string) {
    this.baseUrl = baseUrl ?? env.apiUrl;
  }

  // -- Auth helpers ---------------------------------------------------------

  setToken(token: string | null): void {
    this.token = token;
  }

  setTenantId(tenantId: string | null): void {
    this.tenantId = tenantId;
  }

  setBaseUrl(url: string): void {
    this.baseUrl = url;
  }

  // -- HTTP verbs -----------------------------------------------------------

  async get<T>(path: string): Promise<ApiResponse<T>> {
    return this.request<ApiResponse<T>>("GET", path);
  }

  async getList<T>(path: string): Promise<ApiListResponse<T>> {
    return this.request<ApiListResponse<T>>("GET", path);
  }

  async post<T>(path: string, body: unknown): Promise<ApiResponse<T>> {
    return this.request<ApiResponse<T>>("POST", path, body);
  }

  async patch<T>(path: string, body: unknown): Promise<ApiResponse<T>> {
    return this.request<ApiResponse<T>>("PATCH", path, body);
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

    if (this.tenantId) {
      headers["X-Tenant-ID"] = this.tenantId;
    }

    const url = `${this.baseUrl}${path}`;

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
