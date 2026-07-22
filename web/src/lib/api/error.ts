import type { ApiErrorBody } from "./types";

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
