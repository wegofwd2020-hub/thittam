export interface ApiResponseMeta {
  request_id: string;
  timestamp: string;
}

export interface ApiResponse<T> {
  data: T;
  meta: ApiResponseMeta;
}

export interface PaginationInfo {
  next_cursor: string;
  has_more: boolean;
  total_count: number;
}

export interface ApiListResponse<T> {
  data: T[];
  pagination: PaginationInfo;
}

export interface ApiErrorBody {
  code: string;
  message: string;
  details?: Record<string, unknown>;
  request_id: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
}
