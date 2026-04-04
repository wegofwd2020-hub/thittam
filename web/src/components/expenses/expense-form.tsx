"use client";

import { useCallback, useMemo, useState } from "react";
import type { ExpenseCategory, PurchaseOrder } from "@/lib/api/expenses";
import type { BudgetLineItem } from "@/lib/api/budgets";
import { TaxTreatmentBadge } from "./tax-treatment-badge";

// ---------------------------------------------------------------------------
// ExpenseForm — submission form for new expenses.
// ---------------------------------------------------------------------------

interface ExpenseFormProps {
  productionId: string;
  categories: ExpenseCategory[];
  purchaseOrders: PurchaseOrder[];
  budgetLineItems: BudgetLineItem[];
  onSubmit: (data: {
    category_id: string;
    description: string;
    amount: string;
    budget_line_id?: string;
    purchase_order_id?: string;
  }) => void;
  isSubmitting?: boolean;
}

export function ExpenseForm({
  productionId: _productionId,
  categories,
  purchaseOrders,
  budgetLineItems,
  onSubmit,
  isSubmitting = false,
}: ExpenseFormProps) {
  const [categoryId, setCategoryId] = useState("");
  const [description, setDescription] = useState("");
  const [amount, setAmount] = useState("");
  const [budgetLineId, setBudgetLineId] = useState("");
  const [purchaseOrderId, setPurchaseOrderId] = useState("");

  const selectedCategory = useMemo(
    () => categories.find((c) => c.id === categoryId) ?? null,
    [categories, categoryId],
  );

  const openPurchaseOrders = useMemo(
    () =>
      purchaseOrders.filter(
        (po) => po.status === "approved" || po.status === "partially_invoiced",
      ),
    [purchaseOrders],
  );

  const canSubmit =
    categoryId !== "" &&
    description.trim() !== "" &&
    amount.trim() !== "" &&
    parseFloat(amount) > 0 &&
    (!selectedCategory?.requires_po || purchaseOrderId !== "");

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (!canSubmit) return;

      onSubmit({
        category_id: categoryId,
        description: description.trim(),
        amount,
        budget_line_id: budgetLineId || undefined,
        purchase_order_id: purchaseOrderId || undefined,
      });
    },
    [
      canSubmit,
      categoryId,
      description,
      amount,
      budgetLineId,
      purchaseOrderId,
      onSubmit,
    ],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {/* Category */}
      <div>
        <label
          htmlFor="expense-category"
          className="block text-sm font-heading"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Category
        </label>
        <select
          id="expense-category"
          value={categoryId}
          onChange={(e) => {
            setCategoryId(e.target.value);
            setPurchaseOrderId("");
          }}
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-body focus:outline-none focus:ring-2"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
            backgroundColor: "var(--thittam-card, #ffffff)",
          }}
        >
          <option value="">Select a category</option>
          {categories.map((cat) => (
            <option key={cat.id} value={cat.id}>
              {cat.label}
            </option>
          ))}
        </select>

        {/* Category metadata badges */}
        {selectedCategory && (
          <div className="mt-2 flex items-center gap-2">
            <TaxTreatmentBadge treatment={selectedCategory.tax_treatment} />
            {selectedCategory.requires_po && (
              <span
                className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-heading"
                style={{
                  backgroundColor: "rgb(254 242 232)", // orange-50
                  color: "rgb(154 52 18)", // orange-800
                }}
              >
                PO Required
              </span>
            )}
          </div>
        )}
      </div>

      {/* Amount */}
      <div>
        <label
          htmlFor="expense-amount"
          className="block text-sm font-heading"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Amount
        </label>
        <input
          id="expense-amount"
          type="text"
          inputMode="decimal"
          value={amount}
          onChange={(e) => {
            // Allow digits, one decimal point, up to 2 decimal places
            const val = e.target.value;
            if (val === "" || /^\d+\.?\d{0,2}$/.test(val)) {
              setAmount(val);
            }
          }}
          placeholder="0.00"
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-mono tabular-nums placeholder:text-gray-400 focus:outline-none focus:ring-2"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
            backgroundColor: "var(--thittam-card, #ffffff)",
          }}
        />
      </div>

      {/* Description */}
      <div>
        <label
          htmlFor="expense-description"
          className="block text-sm font-heading"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Description
        </label>
        <textarea
          id="expense-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Describe the expense..."
          rows={3}
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-body placeholder:text-gray-400 focus:outline-none focus:ring-2"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
            backgroundColor: "var(--thittam-card, #ffffff)",
          }}
        />
      </div>

      {/* Purchase Order selector — only when category requires PO */}
      {selectedCategory?.requires_po && (
        <div>
          <label
            htmlFor="expense-po"
            className="block text-sm font-heading"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Purchase Order
          </label>
          <select
            id="expense-po"
            value={purchaseOrderId}
            onChange={(e) => setPurchaseOrderId(e.target.value)}
            className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-body focus:outline-none focus:ring-2"
            style={{
              borderColor: "var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
              backgroundColor: "var(--thittam-card, #ffffff)",
            }}
          >
            <option value="">Select a purchase order</option>
            {openPurchaseOrders.map((po) => (
              <option key={po.id} value={po.id}>
                {po.po_number} — {po.vendor_name} (
                <span className="font-mono">{po.amount}</span>)
              </option>
            ))}
          </select>
          {openPurchaseOrders.length === 0 && (
            <p
              className="mt-1 text-xs font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              No open purchase orders available for this production.
            </p>
          )}
        </div>
      )}

      {/* Budget Line selector */}
      <div>
        <label
          htmlFor="expense-budget-line"
          className="block text-sm font-heading"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Budget Line (optional)
        </label>
        <select
          id="expense-budget-line"
          value={budgetLineId}
          onChange={(e) => setBudgetLineId(e.target.value)}
          className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-body focus:outline-none focus:ring-2"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
            backgroundColor: "var(--thittam-card, #ffffff)",
          }}
        >
          <option value="">No budget line</option>
          {budgetLineItems.map((line) => (
            <option key={line.id} value={line.id}>
              {line.description} ({line.account_code})
            </option>
          ))}
        </select>
      </div>

      {/* Receipt upload placeholder */}
      <div>
        <label
          className="block text-sm font-heading"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Receipt
        </label>
        <div
          className="mt-1 flex items-center justify-center rounded-md border-2 border-dashed px-6 py-8"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <div className="text-center">
            <p
              className="text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Drag and drop a receipt, or click to browse
            </p>
            <p
              className="mt-1 text-xs font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              PDF, JPG, PNG up to 10MB
            </p>
          </div>
        </div>
      </div>

      {/* Submit */}
      <button
        type="submit"
        disabled={!canSubmit || isSubmitting}
        className="w-full rounded-md bg-green-600 px-4 py-2.5 text-sm font-heading text-white transition-colors hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {isSubmitting ? "Submitting..." : "Submit Expense"}
      </button>
    </form>
  );
}
