import { api } from "./client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface DemoUser {
  name: string;
  email: string;
  role: string;
  department: string;
  temp_password: string;
}

export interface DemoProject {
  title: string;
  description: string;
  phase: string;
  budget_total: string;
  expense_count: number;
  status: string;
}

export interface DemoPreview {
  preview_token: string;
  tenant: { name: string; slug: string; plan: string };
  vertical_id: string;
  vertical_name: string;
  users: DemoUser[];
  chart_of_accounts: { code: string; name: string; account_type: string }[];
  budget_categories: { id: string; label: string; description: string }[];
  budget_templates: {
    name: string;
    description: string;
    line_items: { category_id: string; description: string; default_amount: string }[];
  }[];
  expense_categories: {
    id: string;
    label: string;
    tax_treatment: string;
    requires_po: boolean;
  }[];
  approval_workflow: {
    limits: { role: string; max_amount: string }[];
    dual_approval_above: string;
  };
  inventory_categories: { id: string; label: string; is_trackable: boolean }[];
  projects: DemoProject[];
  entity_labels: {
    project: string;
    projectPlural: string;
    phase: string;
    teamMember: string;
  };
}

export interface ProvisionResult {
  tenant_id: string;
  admin_email: string;
  admin_password: string;
  user_count: number;
  project_count: number;
  created_at: string;
}

export interface DemoTenantInfo {
  tenant_id: string;
  name: string;
  vertical_id: string;
  plan: string;
  user_count: number;
  created_at: string;
}

export interface TenantSummary {
  id: string;
  name: string;
  slug: string;
  plan: string;
  vertical_id: string;
  status: string;
  user_count: number;
  created_at: string;
}

export interface VerticalSummary {
  id: string;
  name: string;
  version: string;
  is_active: boolean;
  tenant_count: number;
}

// ---------------------------------------------------------------------------
// Demo provisioning
// ---------------------------------------------------------------------------

const PLATFORM_BASE = "/api/v1/platform";

export async function generateDemoPreview(
  verticalId: string,
  companyName?: string,
  plan?: string,
): Promise<DemoPreview> {
  return api.post<DemoPreview>(`${PLATFORM_BASE}/demo/preview`, {
    vertical_id: verticalId,
    company_name: companyName,
    plan,
  });
}

export async function provisionDemo(
  previewToken: string,
  verticalId: string,
  companyName: string,
  plan: string,
): Promise<ProvisionResult> {
  return api.post<ProvisionResult>(`${PLATFORM_BASE}/demo/provision`, {
    preview_token: previewToken,
    vertical_id: verticalId,
    company_name: companyName,
    plan,
  });
}

export async function listDemoTenants(): Promise<DemoTenantInfo[]> {
  return api.getList<DemoTenantInfo[]>(`${PLATFORM_BASE}/demo/tenants`);
}

export async function teardownDemoTenant(tenantId: string): Promise<void> {
  await api.delete(`${PLATFORM_BASE}/demo/tenants/${tenantId}`);
}

// ---------------------------------------------------------------------------
// Tenant management
// ---------------------------------------------------------------------------

export async function listTenants(): Promise<TenantSummary[]> {
  return api.getList<TenantSummary[]>(`${PLATFORM_BASE}/tenants`);
}

export async function suspendTenant(id: string): Promise<TenantSummary> {
  return api.post<TenantSummary>(
    `${PLATFORM_BASE}/tenants/${id}/suspend`,
    {},
  );
}

export async function reactivateTenant(id: string): Promise<TenantSummary> {
  return api.post<TenantSummary>(
    `${PLATFORM_BASE}/tenants/${id}/reactivate`,
    {},
  );
}

export async function upgradePlan(
  id: string,
  plan: string,
): Promise<TenantSummary> {
  return api.post<TenantSummary>(
    `${PLATFORM_BASE}/tenants/${id}/upgrade`,
    { plan },
  );
}

// ---------------------------------------------------------------------------
// Vertical management
// ---------------------------------------------------------------------------

export async function listVerticals(): Promise<VerticalSummary[]> {
  return api.getList<VerticalSummary[]>(`${PLATFORM_BASE}/verticals`);
}

export async function deprecateVertical(id: string): Promise<VerticalSummary> {
  return api.post<VerticalSummary>(
    `${PLATFORM_BASE}/verticals/${id}/deprecate`,
    {},
  );
}
