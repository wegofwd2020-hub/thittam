"use client";

import { useState, useMemo } from "react";
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

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type BudgetStatus = "draft" | "submitted" | "approved" | "locked";

interface LineItem {
  id: string;
  category: string;
  description: string;
  accountCode: string;
  budgetedAmount: string;
}

interface BudgetDetail {
  id: string;
  label: string;
  productionId: string;
  productionTitle: string;
  status: BudgetStatus;
  totalAmount: string;
  currency: string;
  createdAt: string;
  lineItems: LineItem[];
}

// ---------------------------------------------------------------------------
// Mock data — "The Last Horizon V1" with 13 line items matching XYZ_CBA seed
// ---------------------------------------------------------------------------
const mockBudgets: Record<string, BudgetDetail> = {
  b1: {
    id: "b1",
    label: "The Last Horizon V1",
    productionId: "1",
    productionTitle: "The Last Horizon",
    status: "approved",
    totalAmount: "85000000.00",
    currency: "INR",
    createdAt: "2026-01-20",
    lineItems: [
      { id: "li-01", category: "Above the Line", description: "Director", accountCode: "1100", budgetedAmount: "15000000.00" },
      { id: "li-02", category: "Above the Line", description: "Lead Actor", accountCode: "1200", budgetedAmount: "20000000.00" },
      { id: "li-03", category: "Above the Line", description: "Lead Actress", accountCode: "1201", budgetedAmount: "12000000.00" },
      { id: "li-04", category: "Above the Line", description: "Screenwriter", accountCode: "1300", budgetedAmount: "3000000.00" },
      { id: "li-05", category: "Below the Line", description: "Cinematographer", accountCode: "2100", budgetedAmount: "5000000.00" },
      { id: "li-06", category: "Below the Line", description: "Art Director", accountCode: "2200", budgetedAmount: "3500000.00" },
      { id: "li-07", category: "Below the Line", description: "Camera Equipment", accountCode: "2300", budgetedAmount: "4500000.00" },
      { id: "li-08", category: "Below the Line", description: "Locations", accountCode: "2400", budgetedAmount: "6000000.00" },
      { id: "li-09", category: "Below the Line", description: "Crew", accountCode: "2500", budgetedAmount: "8000000.00" },
      { id: "li-10", category: "Below the Line", description: "Transport", accountCode: "2600", budgetedAmount: "2000000.00" },
      { id: "li-11", category: "Post Production", description: "VFX", accountCode: "3100", budgetedAmount: "4000000.00" },
      { id: "li-12", category: "Post Production", description: "Sound Design", accountCode: "3200", budgetedAmount: "1500000.00" },
      { id: "li-13", category: "Post Production", description: "Music & Score", accountCode: "3300", budgetedAmount: "500000.00" },
    ],
  },
  b2: {
    id: "b2",
    label: "The Last Horizon V2",
    productionId: "1",
    productionTitle: "The Last Horizon",
    status: "draft",
    totalAmount: "92000000.00",
    currency: "INR",
    createdAt: "2026-03-10",
    lineItems: [
      { id: "li-21", category: "Above the Line", description: "Director", accountCode: "1100", budgetedAmount: "15000000.00" },
      { id: "li-22", category: "Above the Line", description: "Lead Actor", accountCode: "1200", budgetedAmount: "22000000.00" },
      { id: "li-23", category: "Post Production", description: "VFX (Revised)", accountCode: "3100", budgetedAmount: "8000000.00" },
    ],
  },
  b3: {
    id: "b3",
    label: "Midnight Express V1",
    productionId: "2",
    productionTitle: "Midnight Express Reboot",
    status: "locked",
    totalAmount: "120000000.00",
    currency: "INR",
    createdAt: "2025-06-15",
    lineItems: [
      { id: "li-31", category: "Above the Line", description: "Director", accountCode: "1100", budgetedAmount: "18000000.00" },
      { id: "li-32", category: "Below the Line", description: "Crew & Equipment", accountCode: "2100", budgetedAmount: "45000000.00" },
    ],
  },
  b4: {
    id: "b4",
    label: "Midnight Express V2",
    productionId: "2",
    productionTitle: "Midnight Express Reboot",
    status: "submitted",
    totalAmount: "125000000.00",
    currency: "INR",
    createdAt: "2025-12-01",
    lineItems: [
      { id: "li-41", category: "Above the Line", description: "Director", accountCode: "1100", budgetedAmount: "18000000.00" },
      { id: "li-42", category: "Below the Line", description: "Crew & Equipment", accountCode: "2100", budgetedAmount: "48000000.00" },
    ],
  },
  b5: {
    id: "b5",
    label: "Project Starfall V1",
    productionId: "3",
    productionTitle: "Project Starfall",
    status: "draft",
    totalAmount: "40000000.00",
    currency: "INR",
    createdAt: "2026-03-05",
    lineItems: [
      { id: "li-51", category: "Above the Line", description: "Director", accountCode: "1100", budgetedAmount: "8000000.00" },
      { id: "li-52", category: "Below the Line", description: "Animation Team", accountCode: "2100", budgetedAmount: "20000000.00" },
    ],
  },
};

// ---------------------------------------------------------------------------
// Budget categories for add line item form
// ---------------------------------------------------------------------------
const BUDGET_CATEGORIES = [
  { name: "Above the Line", defaultCode: "1100" },
  { name: "Below the Line", defaultCode: "2100" },
  { name: "Post Production", defaultCode: "3100" },
  { name: "Marketing & Distribution", defaultCode: "4100" },
  { name: "Insurance & Legal", defaultCode: "5100" },
  { name: "Contingency", defaultCode: "9100" },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function BudgetDetailPage() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();

  const [budget, setBudget] = useState<BudgetDetail | null>(
    () => mockBudgets[params.id] ?? null
  );
  const [showApprovalDialog, setShowApprovalDialog] = useState(false);
  const [approvalAction, setApprovalAction] = useState<"approve" | "reject">("approve");
  const [approvalNote, setApprovalNote] = useState("");
  const [showAddForm, setShowAddForm] = useState(false);

  // Add line item form state
  const [newCategory, setNewCategory] = useState(BUDGET_CATEGORIES[0].name);
  const [newDescription, setNewDescription] = useState("");
  const [newAccountCode, setNewAccountCode] = useState(BUDGET_CATEGORIES[0].defaultCode);
  const [newAmount, setNewAmount] = useState("");

  // Computed totals by category
  const categoryTotals = useMemo(() => {
    if (!budget) return {};
    const totals: Record<string, number> = {};
    for (const li of budget.lineItems) {
      totals[li.category] = (totals[li.category] ?? 0) + parseFloat(li.budgetedAmount);
    }
    return totals;
  }, [budget]);

  const grandTotal = useMemo(() => {
    if (!budget) return 0;
    return budget.lineItems.reduce((sum, li) => sum + parseFloat(li.budgetedAmount), 0);
  }, [budget]);

  if (!budget) {
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
          The budget you are looking for does not exist or has been removed.
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

  function handleStatusChange(newStatus: BudgetStatus) {
    setBudget((prev) => (prev ? { ...prev, status: newStatus } : prev));
    setShowApprovalDialog(false);
    setApprovalNote("");
  }

  function handleAddLineItem(e: React.FormEvent) {
    e.preventDefault();
    if (!newDescription.trim() || !newAmount.trim()) return;

    const newItem: LineItem = {
      id: `li-new-${Date.now()}`,
      category: newCategory,
      description: newDescription,
      accountCode: newAccountCode,
      budgetedAmount: (parseFloat(newAmount) * 100).toFixed(2), // Treat input as "in lakhs" -> store as raw
    };

    // Store the raw amount entered (no multiplication — user enters full amount in rupees)
    newItem.budgetedAmount = parseFloat(newAmount).toFixed(2);

    setBudget((prev) =>
      prev ? { ...prev, lineItems: [...prev.lineItems, newItem] } : prev
    );
    setNewDescription("");
    setNewAmount("");
    setShowAddForm(false);
  }

  function handleCategoryChange(cat: string) {
    setNewCategory(cat);
    const match = BUDGET_CATEGORIES.find((c) => c.name === cat);
    if (match) setNewAccountCode(match.defaultCode);
  }

  // Group line items by category for display
  const categories = useMemo(() => {
    const map = new Map<string, LineItem[]>();
    for (const li of budget.lineItems) {
      const items = map.get(li.category) ?? [];
      items.push(li);
      map.set(li.category, items);
    }
    return Array.from(map.entries());
  }, [budget.lineItems]);

  return (
    <div className="mx-auto max-w-5xl">
      {/* Back link */}
      <Link
        href="/budgets"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Budgets
      </Link>

      {/* Header */}
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

        {/* Action buttons based on status */}
        <div className="flex items-center gap-2">
          {isDraft && (
            <button
              onClick={() => handleStatusChange("submitted")}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <Send className="h-4 w-4" />
              Submit for Approval
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
            <button
              onClick={() => handleStatusChange("locked")}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium transition-colors hover:bg-gray-100"
              style={{
                color: "var(--thittam-foreground, #0f172a)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <Lock className="h-4 w-4" />
              Lock Budget
            </button>
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

      {/* Meta info row */}
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
            href={`/productions/${budget.productionId}`}
            className="text-sm font-body hover:underline"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            {budget.productionTitle}
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
            {budget.createdAt}
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

      {/* Budget summary card */}
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

      {/* Line items table */}
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
            Line Items ({budget.lineItems.length})
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

        {/* Grouped by category */}
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
                        Budgeted Amount
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
                          {li.accountCode}
                        </td>
                        <td className="px-4 py-2.5 text-right">
                          <AmountDisplay
                            amount={li.budgetedAmount}
                            currency={budget.currency}
                            size="sm"
                          />
                        </td>
                      </tr>
                    ))}
                    {/* Category subtotal */}
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
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>

        {/* Grand total row */}
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

      {/* ── Add Line Item slide-out ─────────────────────────────────────── */}
      {showAddForm && (
        <div className="fixed inset-0 z-50 flex justify-end">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/30"
            onClick={() => setShowAddForm(false)}
          />
          {/* Panel */}
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
              {/* Category */}
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
                    <option key={c.name} value={c.name}>
                      {c.name}
                    </option>
                  ))}
                </select>
              </div>

              {/* Description */}
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
                  placeholder="e.g. Stunt Coordinator"
                  className="w-full rounded-lg px-3 py-2 text-sm font-body"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>

              {/* Account code */}
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
                <p
                  className="mt-1 text-xs font-body"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Auto-filled from category default. You can override it.
                </p>
              </div>

              {/* Budgeted amount */}
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

              {/* Actions */}
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
                  disabled={!newDescription.trim() || !newAmount.trim()}
                  className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
                  style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
                >
                  Add Item
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ── Approval Dialog ─────────────────────────────────────────────── */}
      {showApprovalDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/30"
            onClick={() => setShowApprovalDialog(false)}
          />
          {/* Dialog */}
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
                ? `You are about to approve "${budget.label}". This will allow the budget to be locked for execution.`
                : `You are about to reject "${budget.label}". The budget will return to draft status for revision.`}
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
                  handleStatusChange(
                    approvalAction === "approve" ? "approved" : "draft"
                  )
                }
                className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
                style={{
                  backgroundColor:
                    approvalAction === "approve"
                      ? "var(--thittam-status-on-track, #16A34A)"
                      : "var(--thittam-status-over-budget, #DC2626)",
                }}
              >
                {approvalAction === "approve" ? "Approve" : "Reject"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
