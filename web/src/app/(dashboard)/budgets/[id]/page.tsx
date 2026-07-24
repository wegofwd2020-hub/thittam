"use client";

import { useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Plus,
  Lock,
  Send,
  CheckCircle2,
  XCircle,
  X,
} from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import {
  useBudget,
  useLineItems,
  useSubmitBudget,
  useApproveBudget,
  useCreateLineItem,
} from "@/lib/hooks/use-budgets";
import { useProduction } from "@/lib/hooks/use-productions";
import type { BudgetLineItem } from "@/lib/api/budgets";

// ---------------------------------------------------------------------------
// Category label translation — category_id comes back as a short machine
// identifier (e.g. "preliminaries"). Title-case it for display; when a
// vertical-config lookup lands (getBudgetCategories) this can be replaced
// with the real label.
// ---------------------------------------------------------------------------
function formatCategory(id: string): string {
  return id
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

// Display fallback for when the "Add line item" form hasn't been wired
// to a full vertical-aware category list yet. Still covers the six
// construction categories from construction.yaml.
const BUDGET_CATEGORIES = [
  { id: "preliminaries", label: "Preliminaries", defaultCode: "5500" },
  { id: "materials", label: "Materials", defaultCode: "5100" },
  { id: "labour", label: "Labour", defaultCode: "5200" },
  { id: "subcontract", label: "Subcontract", defaultCode: "5300" },
  { id: "plant_equipment", label: "Plant & Equipment", defaultCode: "5400" },
  { id: "contingency", label: "Contingency", defaultCode: "5600" },
];

export default function BudgetDetailPage() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();
  const budgetId = params.id;

  const budgetQuery = useBudget(budgetId);
  const lineItemsQuery = useLineItems(budgetId);

  const productionId = budgetQuery.data?.production_id ?? "";
  const productionQuery = useProduction(productionId);

  const submitMutation = useSubmitBudget();
  const approveMutation = useApproveBudget();
  const createLineItem = useCreateLineItem(budgetId);

  const [showApprovalDialog, setShowApprovalDialog] = useState(false);
  const [approvalAction, setApprovalAction] = useState<"approve" | "reject">("approve");
  const [approvalNote, setApprovalNote] = useState("");
  const [showAddForm, setShowAddForm] = useState(false);

  const [newCategory, setNewCategory] = useState(BUDGET_CATEGORIES[0].id);
  const [newDescription, setNewDescription] = useState("");
  const [newAccountCode, setNewAccountCode] = useState(BUDGET_CATEGORIES[0].defaultCode);
  const [newAmount, setNewAmount] = useState("");

  const budget = budgetQuery.data;
  const lineItems: BudgetLineItem[] = useMemo(
    () => lineItemsQuery.data ?? [],
    [lineItemsQuery.data],
  );

  const categoryTotals = useMemo(() => {
    const totals: Record<string, number> = {};
    for (const li of lineItems) {
      const key = formatCategory(li.category_id);
      totals[key] = (totals[key] ?? 0) + parseFloat(li.budgeted_amount);
    }
    return totals;
  }, [lineItems]);

  const grandTotal = useMemo(
    () => lineItems.reduce((sum, li) => sum + parseFloat(li.budgeted_amount), 0),
    [lineItems],
  );

  // Group line items by category for display
  const categories = useMemo(() => {
    const map = new Map<string, BudgetLineItem[]>();
    for (const li of lineItems) {
      const key = formatCategory(li.category_id);
      const items = map.get(key) ?? [];
      items.push(li);
      map.set(key, items);
    }
    return Array.from(map.entries());
  }, [lineItems]);

  // ── Loading / not-found states ────────────────────────────────────────
  if (budgetQuery.isLoading) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <p
          className="font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Loading budget…
        </p>
      </div>
    );
  }

  if (budgetQuery.error || !budget) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <h1
          className="font-heading text-xl font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Budget not found
        </h1>
        <p
          className="mt-2 font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {budgetQuery.error?.message ??
            "The budget you are looking for does not exist or has been removed."}
        </p>
        <Link
          href="/budgets"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Budgets
        </Link>
      </div>
    );
  }

  const isDraft = budget.status === "draft";
  const isSubmitted = budget.status === "submitted";
  const isApproved = budget.status === "approved";
  const isLocked = budget.status === "locked";

  function handleAddLineItem(e: React.FormEvent) {
    e.preventDefault();
    if (!newDescription.trim() || !newAmount.trim()) return;

    createLineItem.mutate(
      {
        category_id: newCategory,
        description: newDescription,
        account_code: newAccountCode,
        budgeted_amount: parseFloat(newAmount).toFixed(2),
      },
      {
        onSuccess: () => {
          setNewDescription("");
          setNewAmount("");
          setShowAddForm(false);
        },
      },
    );
  }

  function handleCategoryChange(catId: string) {
    setNewCategory(catId);
    const match = BUDGET_CATEGORIES.find((c) => c.id === catId);
    if (match) setNewAccountCode(match.defaultCode);
  }

  return (
    <div className="mx-auto max-w-5xl">
      <Link
        href="/budgets"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Budgets
      </Link>

      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <h1
            className="font-heading text-2xl font-semibold tracking-tight"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {budget.label}
          </h1>
          <StatusBadge status={budget.status} variant="budget" />
        </div>

        <div className="flex items-center gap-2">
          {isDraft && (
            <button
              onClick={() => submitMutation.mutate(budget.id)}
              disabled={submitMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <Send className="h-4 w-4" />
              {submitMutation.isPending ? "Submitting…" : "Submit for Approval"}
            </button>
          )}
          {isSubmitted && (
            <>
              <button
                onClick={() => {
                  setApprovalAction("approve");
                  setShowApprovalDialog(true);
                }}
                className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
                style={{ backgroundColor: "var(--thittam-status-on-track, #16A34A)" }}
              >
                <CheckCircle2 className="h-4 w-4" />
                Approve
              </button>
              <button
                onClick={() => {
                  setApprovalAction("reject");
                  setShowApprovalDialog(true);
                }}
                className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
                style={{ backgroundColor: "var(--thittam-status-over-budget, #DC2626)" }}
              >
                <XCircle className="h-4 w-4" />
                Reject
              </button>
            </>
          )}
          {isApproved && (
            <span
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-heading font-medium"
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                color: "var(--thittam-muted-foreground, #64748b)",
              }}
            >
              <Lock className="h-3.5 w-3.5" />
              Approved — lock action not yet available via API
            </span>
          )}
          {isLocked && (
            <span
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-heading font-medium"
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                color: "var(--thittam-muted-foreground, #64748b)",
              }}
            >
              <Lock className="h-3.5 w-3.5" />
              Read-only
            </span>
          )}
        </div>
      </div>

      <div
        className="mb-6 flex flex-wrap gap-6 rounded-xl p-4"
        style={{
          backgroundColor: "var(--thittam-muted, #f1f5f9)",
        }}
      >
        <div>
          <span
            className="block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {entityLabels.project}
          </span>
          <Link
            href={`/productions/${budget.production_id}`}
            className="text-sm font-body hover:underline"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            {productionQuery.data?.title ?? budget.production_id}
          </Link>
        </div>
        <div>
          <span
            className="block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Created
          </span>
          <span
            className="text-sm font-mono tabular-nums"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {budget.created_at.slice(0, 10)}
          </span>
        </div>
        <div>
          <span
            className="block text-xs font-heading font-medium"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Currency
          </span>
          <span
            className="text-sm font-mono tabular-nums"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {budget.currency}
          </span>
        </div>
      </div>

      <div
        className="mb-6 rounded-xl p-5"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <h2
          className="mb-4 font-heading text-sm font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Budget Summary
        </h2>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Object.entries(categoryTotals).map(([cat, total]) => (
            <div key={cat}>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                {cat}
              </span>
              <AmountDisplay amount={total.toFixed(2)} currency={budget.currency} size="sm" />
            </div>
          ))}
          <div
            className="rounded-lg p-3"
            style={{
              backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 8%, transparent)",
            }}
          >
            <span
              className="block text-xs font-heading font-semibold"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            >
              Grand Total
            </span>
            <AmountDisplay amount={grandTotal.toFixed(2)} currency={budget.currency} size="lg" />
          </div>
        </div>
      </div>

      <div
        className="rounded-xl p-5"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2
            className="font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Line Items ({lineItems.length})
          </h2>
          {isDraft && (
            <button
              onClick={() => setShowAddForm(true)}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <Plus className="h-4 w-4" />
              Add Line Item
            </button>
          )}
        </div>

        {lineItemsQuery.isLoading ? (
          <p
            className="py-8 text-center font-body text-sm"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Loading line items…
          </p>
        ) : lineItems.length === 0 ? (
          <p
            className="py-8 text-center font-body text-sm"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            No line items yet.
          </p>
        ) : (
          <div className="flex flex-col gap-4">
            {categories.map(([category, items]) => (
              <div key={category}>
                <h3
                  className="mb-2 text-xs font-heading font-semibold uppercase tracking-wider"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  {category}
                </h3>
                <div
                  className="overflow-x-auto rounded-lg border"
                  style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
                >
                  <table className="w-full text-sm">
                    <thead>
                      <tr
                        style={{
                          backgroundColor: "var(--thittam-muted, #f1f5f9)",
                          borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                        }}
                      >
                        <th
                          className="px-4 py-2.5 text-left text-xs font-heading font-semibold uppercase tracking-wider"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        >
                          Description
                        </th>
                        <th
                          className="px-4 py-2.5 text-left text-xs font-heading font-semibold uppercase tracking-wider"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        >
                          Account Code
                        </th>
                        <th
                          className="px-4 py-2.5 text-right text-xs font-heading font-semibold uppercase tracking-wider"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        >
                          Budgeted
                        </th>
                        <th
                          className="px-4 py-2.5 text-right text-xs font-heading font-semibold uppercase tracking-wider"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        >
                          Actual
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {items.map((li) => (
                        <tr
                          key={li.id}
                          style={{
                            borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                          }}
                        >
                          <td
                            className="px-4 py-2.5 font-body"
                            style={{ color: "var(--thittam-foreground, #0f172a)" }}
                          >
                            {li.description}
                          </td>
                          <td
                            className="px-4 py-2.5 font-mono tabular-nums text-sm"
                            style={{ color: "var(--thittam-foreground, #0f172a)" }}
                          >
                            {li.account_code}
                          </td>
                          <td className="px-4 py-2.5 text-right">
                            <AmountDisplay
                              amount={li.budgeted_amount}
                              currency={budget.currency}
                              size="sm"
                            />
                          </td>
                          <td className="px-4 py-2.5 text-right">
                            <AmountDisplay
                              amount={li.actual_amount}
                              currency={budget.currency}
                              size="sm"
                            />
                          </td>
                        </tr>
                      ))}
                      <tr
                        style={{
                          backgroundColor: "var(--thittam-muted, #f1f5f9)",
                        }}
                      >
                        <td
                          colSpan={2}
                          className="px-4 py-2 text-right text-xs font-heading font-semibold uppercase"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        >
                          Subtotal
                        </td>
                        <td className="px-4 py-2 text-right">
                          <AmountDisplay
                            amount={(categoryTotals[category] ?? 0).toFixed(2)}
                            currency={budget.currency}
                            size="sm"
                          />
                        </td>
                        <td className="px-4 py-2 text-right">
                          <AmountDisplay
                            amount={items
                              .reduce((s, it) => s + parseFloat(it.actual_amount), 0)
                              .toFixed(2)}
                            currency={budget.currency}
                            size="sm"
                          />
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
          </div>
        )}

        <div
          className="mt-4 flex items-center justify-between rounded-lg p-4"
          style={{
            backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 8%, transparent)",
          }}
        >
          <span
            className="text-sm font-heading font-semibold"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            Grand Total
          </span>
          <AmountDisplay amount={grandTotal.toFixed(2)} currency={budget.currency} size="lg" />
        </div>
      </div>

      {showAddForm && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div
            className="absolute inset-0 bg-black/30"
            onClick={() => setShowAddForm(false)}
          />
          <div
            className="relative w-full max-w-md overflow-y-auto p-6"
            style={{ backgroundColor: "var(--thittam-background, #fff)" }}
          >
            <div className="mb-6 flex items-center justify-between">
              <h2
                className="font-heading text-lg font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Add Line Item
              </h2>
              <button
                onClick={() => setShowAddForm(false)}
                className="rounded-md p-1 transition-colors hover:bg-gray-100"
              >
                <X className="h-5 w-5" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
              </button>
            </div>

            <form onSubmit={handleAddLineItem} className="flex flex-col gap-5">
              <div>
                <label
                  htmlFor="add-category"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Category <span className="text-red-500">*</span>
                </label>
                <select
                  id="add-category"
                  value={newCategory}
                  onChange={(e) => handleCategoryChange(e.target.value)}
                  className="w-full rounded-lg px-3 py-2 text-sm font-body"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                >
                  {BUDGET_CATEGORIES.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.label}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label
                  htmlFor="add-description"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Description <span className="text-red-500">*</span>
                </label>
                <input
                  id="add-description"
                  type="text"
                  value={newDescription}
                  onChange={(e) => setNewDescription(e.target.value)}
                  placeholder="e.g. Site setup"
                  className="w-full rounded-lg px-3 py-2 text-sm font-body"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>

              <div>
                <label
                  htmlFor="add-account"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Account Code
                </label>
                <input
                  id="add-account"
                  type="text"
                  value={newAccountCode}
                  onChange={(e) => setNewAccountCode(e.target.value)}
                  className="w-full rounded-lg px-3 py-2 text-sm font-mono tabular-nums"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>

              <div>
                <label
                  htmlFor="add-amount"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Budgeted Amount ({budget.currency}) <span className="text-red-500">*</span>
                </label>
                <input
                  id="add-amount"
                  type="text"
                  inputMode="decimal"
                  value={newAmount}
                  onChange={(e) => setNewAmount(e.target.value)}
                  placeholder="0.00"
                  className="w-full rounded-lg px-3 py-2 text-sm font-mono tabular-nums"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>

              <div className="mt-4 flex items-center justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setShowAddForm(false)}
                  className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
                  style={{
                    color: "var(--thittam-muted-foreground, #64748b)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                  }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={
                    !newDescription.trim() ||
                    !newAmount.trim() ||
                    createLineItem.isPending
                  }
                  className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
                  style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
                >
                  {createLineItem.isPending ? "Adding…" : "Add Item"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {showApprovalDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/30"
            onClick={() => setShowApprovalDialog(false)}
          />
          <div
            className="relative w-full max-w-md rounded-xl p-6 shadow-xl"
            style={{ backgroundColor: "var(--thittam-background, #fff)" }}
          >
            <div className="mb-4 flex items-center justify-between">
              <h2
                className="font-heading text-lg font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {approvalAction === "approve" ? "Approve Budget" : "Reject Budget"}
              </h2>
              <button
                onClick={() => setShowApprovalDialog(false)}
                className="rounded-md p-1 transition-colors hover:bg-gray-100"
              >
                <X className="h-5 w-5" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
              </button>
            </div>

            <p
              className="mb-4 font-body text-sm"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {approvalAction === "approve"
                ? `You are about to approve "${budget.label}".`
                : `Reject is not yet wired to the backend — the approve API will be called instead. This dialog remains for UX continuity.`}
            </p>

            <div className="mb-4">
              <label
                htmlFor="approval-note"
                className="mb-1.5 block text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Note (optional)
              </label>
              <textarea
                id="approval-note"
                rows={3}
                value={approvalNote}
                onChange={(e) => setApprovalNote(e.target.value)}
                placeholder="Add a note about this decision..."
                className="w-full resize-y rounded-lg px-3 py-2 text-sm font-body"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                  color: "var(--thittam-foreground, #0f172a)",
                }}
              />
            </div>

            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowApprovalDialog(false)}
                className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
                style={{
                  color: "var(--thittam-muted-foreground, #64748b)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                Cancel
              </button>
              <button
                onClick={() =>
                  approveMutation.mutate(budget.id, {
                    onSuccess: () => setShowApprovalDialog(false),
                  })
                }
                disabled={approveMutation.isPending}
                className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
                style={{
                  backgroundColor:
                    approvalAction === "approve"
                      ? "var(--thittam-status-on-track, #16A34A)"
                      : "var(--thittam-status-over-budget, #DC2626)",
                }}
              >
                {approveMutation.isPending
                  ? "Approving…"
                  : approvalAction === "approve"
                    ? "Approve"
                    : "Approve Anyway"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
