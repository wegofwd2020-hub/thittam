import { api } from "./client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface User {
  id: string;
  tenant_id: string;
  email: string;
  display_name: string;
  status: string; // active | invited | deactivated
  roles: string[];
  created_at: string;
  last_login_at: string | null;
}

export interface Role {
  id: string;
  name: string;
  permissions: string[];
}

export interface AuthConfig {
  auth_method: string; // local | oidc
  issuer_url: string | null;
  client_id: string | null;
  scopes: string[];
  email_claim: string;
  display_name_claim: string;
  groups_claim: string | null;
  auto_provision: boolean;
  default_role: string;
}

export interface AuditLogEntry {
  id: string;
  tenant_id: string;
  actor_id: string;
  actor_email: string;
  actor_ip: string;
  action: string;
  resource_type: string;
  resource_id: string;
  production_id: string | null;
  old_state: unknown;
  new_state: unknown;
  metadata: unknown;
  occurred_at: string;
}

export interface TenantSettings {
  name: string;
  slug: string;
  plan: string;
  vertical_id: string;
  vertical_name: string;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Request payloads
// ---------------------------------------------------------------------------

export interface InviteUserInput {
  email: string;
  role: string;
  department?: string;
}

export interface UpdateUserRoleInput {
  role: string;
}

export interface UpdateAuthConfigInput {
  auth_method: string;
  issuer_url?: string;
  client_id?: string;
  client_secret?: string;
  scopes?: string[];
  email_claim?: string;
  display_name_claim?: string;
  groups_claim?: string;
  auto_provision?: boolean;
  default_role?: string;
}

export interface AuditLogQuery {
  actor_id?: string;
  action?: string;
  resource_type?: string;
  from?: string;
  to?: string;
  limit?: number;
  after?: string;
}

// ---------------------------------------------------------------------------
// Query-string helper
// ---------------------------------------------------------------------------

function qs(params?: Record<string, string | number | undefined>): string {
  if (!params) return "";
  const entries = Object.entries(params).filter(
    ([, v]) => v !== undefined && v !== "",
  );
  if (entries.length === 0) return "";
  const search = new URLSearchParams(
    entries.map(([k, v]) => [k, String(v)]),
  );
  return `?${search.toString()}`;
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

const USERS_BASE = "/api/v1/users";

export async function listUsers(): Promise<User[]> {
  return api.getList<User[]>(USERS_BASE);
}

export async function getUser(id: string): Promise<User> {
  return api.get<User>(`${USERS_BASE}/${id}`);
}

export async function inviteUser(data: InviteUserInput): Promise<User> {
  return api.post<User>(`${USERS_BASE}/invite`, data);
}

export async function updateUserRole(
  id: string,
  data: UpdateUserRoleInput,
): Promise<User> {
  return api.patch<User>(`${USERS_BASE}/${id}/role`, data);
}

export async function deactivateUser(id: string): Promise<User> {
  return api.post<User>(`${USERS_BASE}/${id}/deactivate`, {});
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

const ROLES_BASE = "/api/v1/roles";

export async function listRoles(): Promise<Role[]> {
  return api.getList<Role[]>(ROLES_BASE);
}

// ---------------------------------------------------------------------------
// Auth Config
// ---------------------------------------------------------------------------

const AUTH_BASE = "/api/v1/settings/auth";

export async function getAuthConfig(): Promise<AuthConfig> {
  return api.get<AuthConfig>(AUTH_BASE);
}

export async function updateAuthConfig(
  data: UpdateAuthConfigInput,
): Promise<AuthConfig> {
  return api.patch<AuthConfig>(AUTH_BASE, data);
}

// ---------------------------------------------------------------------------
// Audit Log
// ---------------------------------------------------------------------------

const AUDIT_BASE = "/api/v1/audit-log";

export async function queryAuditLog(
  params?: AuditLogQuery,
): Promise<AuditLogEntry[]> {
  return api.getList<AuditLogEntry[]>(`${AUDIT_BASE}${qs(params as Record<string, string | number | undefined>)}`);
}

// ---------------------------------------------------------------------------
// Tenant Settings
// ---------------------------------------------------------------------------

const TENANT_BASE = "/api/v1/settings/tenant";

export async function getTenantSettings(): Promise<TenantSettings> {
  return api.get<TenantSettings>(TENANT_BASE);
}
