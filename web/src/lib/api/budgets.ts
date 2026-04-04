import { api } from "./client";
import type { ApiListResponse, ApiResponse } from "./types";

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

export async function listBudgets(
  productionId: string,
  params?: { status?: string; limit?: number; after?: string },
): Promise<ApiListResponse<Budget>> {
  return api.getList<Budget>(
    `${BASE}${qs({ production_id: productionId, ...params })}`,
  );
}

export async function getBudget(id: string): Promise<Budget> {
  const res = await api.get<Budget>(`${BASE}/${id}`);
  return res.data;
}

export async function createBudget(
  data: CreateBudgetInput,
): Promise<Budget> {
  const res = await api.post<Budget>(BASE, data);
  return res.data;
}

export async function createBudgetFromTemplate(
  data: CreateBudgetFromTemplateInput,
): Promise<Budget> {
  const res = await api.post<Budget>(`${BASE}/from-template`, data);
  return res.data;
}

export async function submitBudget(id: string): Promise<Budget> {
  const res = await api.post<Budget>(`${BASE}/${id}/submit`, {});
  return res.data;
}

export async function approveBudget(id: string): Promise<Budget> {
  const res = await api.post<Budget>(`${BASE}/${id}/approve`, {});
  return res.data;
}

// ---------------------------------------------------------------------------
// Line Items
// ---------------------------------------------------------------------------

export async function listLineItems(
  budgetId: string,
): Promise<BudgetLineItem[]> {
  const res = await api.getList<BudgetLineItem>(
    `${BASE}/${budgetId}/line-items`,
  );
  return res.data;
}

export async function createLineItem(
  budgetId: string,
  data: CreateLineItemInput,
): Promise<BudgetLineItem> {
  const res = await api.post<BudgetLineItem>(
    `${BASE}/${budgetId}/line-items`,
    data,
  );
  return res.data;
}

export async function updateLineItem(
  id: string,
  data: UpdateLineItemInput,
): Promise<BudgetLineItem> {
  const res = await api.patch<BudgetLineItem>(
    `/api/v1/line-items/${id}`,
    data,
  );
  return res.data;
}

export async function deleteLineItem(id: string): Promise<void> {
  await api.delete(`/api/v1/line-items/${id}`);
}

export async function checkLineAvailability(
  lineId: string,
): Promise<LineItemAvailability> {
  const res = await api.get<LineItemAvailability>(
    `/api/v1/line-items/${lineId}/availability`,
  );
  return res.data;
}

// ---------------------------------------------------------------------------
// Categories & Templates (vertical config)
// ---------------------------------------------------------------------------

export async function getBudgetCategories(): Promise<BudgetCategory[]> {
  const res = await api.getList<BudgetCategory>(
    "/api/v1/config/budget-categories",
  );
  return res.data;
}

export async function getBudgetTemplates(): Promise<BudgetTemplate[]> {
  const res = await api.getList<BudgetTemplate>(
    "/api/v1/config/budget-templates",
  );
  return res.data;
}
