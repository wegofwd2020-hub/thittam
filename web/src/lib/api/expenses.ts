import { api } from "./client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Expense {
  id: string;
  production_id: string;
  tenant_id: string;
  budget_line_id: string | null;
  purchase_order_id: string | null;
  category_id: string;
  description: string;
  amount: string; // decimal string
  currency: string;
  tax_amount: string;
  status: string; // draft | submitted | approved | rejected | paid
  submitted_by: string | null;
  approved_by: string | null;
  submitted_at: string | null;
  approved_at: string | null;
  created_at: string;
}

export interface PurchaseOrder {
  id: string;
  production_id: string;
  tenant_id: string;
  budget_line_id: string | null;
  po_number: string;
  vendor_name: string;
  vendor_gstin: string;
  description: string;
  amount: string;
  currency: string;
  status: string; // draft | approved | partially_invoiced | closed | cancelled
  raised_by: string;
  approved_by: string | null;
  raised_at: string;
  approved_at: string | null;
}

export interface PettyCashAdvance {
  id: string;
  production_id: string;
  tenant_id: string;
  issued_to: string;
  amount: string;
  purpose: string;
  status: string; // issued | partially_settled | settled
  issued_at: string;
  settled_at: string | null;
  unspent_amount: string;
}

export interface ExpenseCategory {
  id: string;
  label: string;
  tax_treatment: string; // input_gst | tds_applicable | none
  default_account_code: string;
  requires_po: boolean;
}

export interface ApprovalLimit {
  role: string;
  max_amount: string;
}

export interface ApprovalWorkflow {
  limits: ApprovalLimit[];
  dual_approval_above: string;
}

// ---------------------------------------------------------------------------
// Request payloads
// ---------------------------------------------------------------------------

export interface SubmitExpenseInput {
  production_id: string;
  category_id: string;
  description: string;
  amount: string;
  currency?: string;
  budget_line_id?: string;
  purchase_order_id?: string;
}

export interface CreatePurchaseOrderInput {
  production_id: string;
  budget_line_id?: string;
  vendor_name: string;
  vendor_gstin?: string;
  description: string;
  amount: string;
  currency?: string;
}

export interface CreatePettyCashInput {
  production_id: string;
  issued_to: string;
  amount: string;
  purpose: string;
}

export interface SettlePettyCashInput {
  unspent_amount: string;
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
// Expenses
// ---------------------------------------------------------------------------

const BASE = "/api/v1/expenses";

export async function listExpenses(
  productionId: string,
  params?: { status?: string; limit?: number; after?: string },
): Promise<Expense[]> {
  return api.getList<Expense[]>(
    `${BASE}${qs({ production_id: productionId, ...params })}`,
  );
}

export async function getExpense(id: string): Promise<Expense> {
  return api.get<Expense>(`${BASE}/${id}`);
}

export async function submitExpense(
  data: SubmitExpenseInput,
): Promise<Expense> {
  return api.post<Expense>(BASE, data);
}

export async function approveExpense(id: string): Promise<Expense> {
  return api.post<Expense>(`${BASE}/${id}/approve`, {});
}

export async function rejectExpense(
  id: string,
  reason: string,
): Promise<Expense> {
  return api.post<Expense>(`${BASE}/${id}/reject`, { reason });
}

// ---------------------------------------------------------------------------
// Purchase Orders
// ---------------------------------------------------------------------------

const PO_BASE = "/api/v1/purchase-orders";

export async function listPurchaseOrders(
  productionId: string,
  params?: { status?: string; limit?: number; after?: string },
): Promise<PurchaseOrder[]> {
  return api.getList<PurchaseOrder[]>(
    `${PO_BASE}${qs({ production_id: productionId, ...params })}`,
  );
}

export async function createPurchaseOrder(
  data: CreatePurchaseOrderInput,
): Promise<PurchaseOrder> {
  return api.post<PurchaseOrder>(PO_BASE, data);
}

export async function approvePurchaseOrder(
  id: string,
): Promise<PurchaseOrder> {
  return api.post<PurchaseOrder>(`${PO_BASE}/${id}/approve`, {});
}

// ---------------------------------------------------------------------------
// Petty Cash
// ---------------------------------------------------------------------------

const PETTY_BASE = "/api/v1/petty-cash";

export async function listPettyCash(
  productionId: string,
  params?: { status?: string; limit?: number; after?: string },
): Promise<PettyCashAdvance[]> {
  return api.getList<PettyCashAdvance[]>(
    `${PETTY_BASE}${qs({ production_id: productionId, ...params })}`,
  );
}

export async function createPettyCash(
  data: CreatePettyCashInput,
): Promise<PettyCashAdvance> {
  return api.post<PettyCashAdvance>(PETTY_BASE, data);
}

export async function settlePettyCash(
  id: string,
  data: SettlePettyCashInput,
): Promise<PettyCashAdvance> {
  return api.post<PettyCashAdvance>(
    `${PETTY_BASE}/${id}/settle`,
    data,
  );
}

// ---------------------------------------------------------------------------
// Categories & Approval config (vertical config)
// ---------------------------------------------------------------------------

export async function getExpenseCategories(): Promise<ExpenseCategory[]> {
  return api.getList<ExpenseCategory[]>(
    "/api/v1/config/expense-categories",
  );
}

export async function getApprovalLimits(): Promise<ApprovalWorkflow> {
  return api.get<ApprovalWorkflow>(
    "/api/v1/config/approval-workflow",
  );
}
