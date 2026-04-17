import { api } from "./client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Budget {
  id: string;
  production_id: string;
  tenant_id: string;
  label: string;
  status: string; // draft | submitted | approved | locked
  currency: string;
  total_amount: string; // decimal string
  submitted_by: string | null;
  approved_by: string | null;
  submitted_at: string | null;
  approved_at: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface BudgetLineItem {
  id: string;
  budget_id: string;
  tenant_id: string;
  category_id: string;
  description: string;
  account_code: string;
  budgeted_amount: string;
  actual_amount: string;
  committed_amount: string;
  is_locked: boolean;
}

export interface BudgetCategory {
  id: string;
  label: string;
  description: string;
  default_account_code: string;
}

export interface BudgetTemplate {
  name: string;
  description: string;
  line_items: {
    category_id: string;
    description: string;
    account_code: string;
    default_amount: string;
    is_required: boolean;
  }[];
}

export interface LineItemAvailability {
  budgeted_amount: string;
  actual_amount: string;
  committed_amount: string;
  available_amount: string;
}

// ---------------------------------------------------------------------------
// Request payloads
// ---------------------------------------------------------------------------

export interface CreateBudgetInput {
  production_id: string;
  label: string;
  currency?: string;
}

export interface CreateBudgetFromTemplateInput {
  production_id: string;
  template_name: string;
  label: string;
}

export interface CreateLineItemInput {
  category_id: string;
  description: string;
  account_code: string;
  budgeted_amount: string;
}

export interface UpdateLineItemInput {
  description?: string;
  account_code?: string;
  budgeted_amount?: string;
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
// Budgets
// ---------------------------------------------------------------------------

const BASE = "/api/v1/budgets";

// grpc-gateway returns list RPCs with the plural field name as the root key
// (ListBudgetsResponse.budgets, not a generic {data: []} envelope). Single-
// entity RPCs return the entity fields flat at the root.

export interface ListBudgetsResponse {
  budgets: Budget[];
}

export interface ListLineItemsResponse {
  line_items: BudgetLineItem[];
}

export interface GetBudgetCategoriesResponse {
  categories: BudgetCategory[];
}

export interface GetBudgetTemplatesResponse {
  templates: BudgetTemplate[];
}

export async function listBudgets(
  productionId: string,
  params?: { status?: string; limit?: number; after?: string },
): Promise<ListBudgetsResponse> {
  return api.getList<ListBudgetsResponse>(
    `${BASE}${qs({ production_id: productionId, ...params })}`,
  );
}

export async function getBudget(id: string): Promise<Budget> {
  return api.get<Budget>(`${BASE}/${id}`);
}

export async function createBudget(
  data: CreateBudgetInput,
): Promise<Budget> {
  return api.post<Budget>(BASE, data);
}

export async function createBudgetFromTemplate(
  data: CreateBudgetFromTemplateInput,
): Promise<Budget> {
  return api.post<Budget>(`${BASE}/from-template`, data);
}

export async function submitBudget(id: string): Promise<Budget> {
  return api.post<Budget>(`${BASE}/${id}/submit`, {});
}

export async function approveBudget(id: string): Promise<Budget> {
  return api.post<Budget>(`${BASE}/${id}/approve`, {});
}

// ---------------------------------------------------------------------------
// Line Items
// ---------------------------------------------------------------------------

export async function listLineItems(
  budgetId: string,
): Promise<BudgetLineItem[]> {
  const res = await api.getList<ListLineItemsResponse>(
    `${BASE}/${budgetId}/line-items`,
  );
  return res.line_items ?? [];
}

export async function createLineItem(
  budgetId: string,
  data: CreateLineItemInput,
): Promise<BudgetLineItem> {
  return api.post<BudgetLineItem>(`${BASE}/${budgetId}/line-items`, data);
}

export async function updateLineItem(
  id: string,
  data: UpdateLineItemInput,
): Promise<BudgetLineItem> {
  return api.patch<BudgetLineItem>(`/api/v1/line-items/${id}`, data);
}

export async function deleteLineItem(id: string): Promise<void> {
  await api.delete(`/api/v1/line-items/${id}`);
}

export async function checkLineAvailability(
  lineId: string,
): Promise<LineItemAvailability> {
  return api.get<LineItemAvailability>(
    `/api/v1/line-items/${lineId}/availability`,
  );
}

// ---------------------------------------------------------------------------
// Categories & Templates (vertical config)
// ---------------------------------------------------------------------------

export async function getBudgetCategories(): Promise<BudgetCategory[]> {
  const res = await api.getList<GetBudgetCategoriesResponse>(
    "/api/v1/config/budget-categories",
  );
  return res.categories ?? [];
}

export async function getBudgetTemplates(): Promise<BudgetTemplate[]> {
  const res = await api.getList<GetBudgetTemplatesResponse>(
    "/api/v1/config/budget-templates",
  );
  return res.templates ?? [];
}
