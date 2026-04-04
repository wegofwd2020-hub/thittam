"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Check, AlertTriangle } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type TaxTreatment = "gst_18" | "gst_12" | "gst_5" | "gst_exempt" | "tds_194c" | "tds_194j";

interface ExpenseCategory {
  name: string;
  taxTreatment: TaxTreatment;
  requiresPo: boolean;
}

// ---------------------------------------------------------------------------
// Tax treatment display config
// ---------------------------------------------------------------------------
const TAX_LABELS: Record<TaxTreatment, { label: string; color: string; bg: string }> = {
  gst_18: { label: "GST 18%", color: "#9333ea", bg: "#f3e8ff" },
  gst_12: { label: "GST 12%", color: "#7c3aed", bg: "#ede9fe" },
  gst_5: { label: "GST 5%", color: "#6d28d9", bg: "#ede9fe" },
  gst_exempt: { label: "GST Exempt", color: "#64748b", bg: "#f1f5f9" },
  tds_194c: { label: "TDS 194C", color: "#0369a1", bg: "#e0f2fe" },
  tds_194j: { label: "TDS 194J", color: "#0e7490", bg: "#cffafe" },
};

function TaxTreatmentBadge({ treatment }: { treatment: TaxTreatment }) {
  const config = TAX_LABELS[treatment];
  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
      style={{ backgroundColor: config.bg, color: config.color }}
    >
      {config.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Mock data — vertical config categories
// ---------------------------------------------------------------------------
const EXPENSE_CATEGORIES: ExpenseCategory[] = [
  { name: "Locations", taxTreatment: "gst_18", requiresPo: true },
  { name: "Camera Equipment", taxTreatment: "gst_18", requiresPo: true },
  { name: "Post Production", taxTreatment: "gst_18", requiresPo: true },
  { name: "Travel & Accommodation", taxTreatment: "gst_18", requiresPo: false },
  { name: "Crew & Catering", taxTreatment: "gst_5", requiresPo: false },
  { name: "Talent Fees", taxTreatment: "tds_194j", requiresPo: true },
  { name: "Contractor Services", taxTreatment: "tds_194c", requiresPo: true },
  { name: "Insurance & Legal", taxTreatment: "gst_exempt", requiresPo: false },
  { name: "Miscellaneous", taxTreatment: "gst_12", requiresPo: false },
];

const mockProductions = [
  { id: "1", title: "The Last Horizon" },
  { id: "2", title: "Midnight Express Reboot" },
  { id: "3", title: "Project Starfall" },
  { id: "4", title: "Urban Legends: Season 2" },
];

const mockPurchaseOrders = [
  { id: "po-001", number: "PO-001", vendor: "Film City Studios", amount: "1800000.00" },
  { id: "po-002", number: "PO-002", vendor: "Goa Locations Pvt Ltd", amount: "500000.00" },
  { id: "po-003", number: "PO-003", vendor: "ARRI Rental India", amount: "2200000.00" },
  { id: "po-004", number: "PO-004", vendor: "Heritage Building Trust", amount: "250000.00" },
  { id: "po-005", number: "PO-005", vendor: "SkyView Drone Services", amount: "180000.00" },
];

const mockBudgetLines = [
  { id: "li-07", label: "Camera Equipment (2300)" },
  { id: "li-08", label: "Locations (2400)" },
  { id: "li-09", label: "Crew (2500)" },
  { id: "li-11", label: "VFX (3100)" },
  { id: "li-12", label: "Sound Design (3200)" },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function NewExpensePage() {
  const { entityLabels } = useTheme();
  const router = useRouter();

  const [selectedProduction, setSelectedProduction] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("");
  const [selectedPo, setSelectedPo] = useState("");
  const [selectedBudgetLine, setSelectedBudgetLine] = useState("");
  const [amount, setAmount] = useState("");
  const [description, setDescription] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showToast, setShowToast] = useState(false);

  const category = EXPENSE_CATEGORIES.find((c) => c.name === selectedCategory);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!selectedProduction || !selectedCategory || !amount.trim() || !description.trim()) return;

    setIsSubmitting(true);
    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 600));
    setIsSubmitting(false);
    setShowToast(true);

    setTimeout(() => {
      router.push("/expenses");
    }, 1200);
  }

  return (
    <div className="mx-auto max-w-2xl">
      {/* Back link */}
      <Link
        href="/expenses"
        className="mb-6 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Expenses
      </Link>

      {/* Page heading */}
      <h1
        className="mb-1 font-heading text-2xl font-semibold tracking-tight"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        Submit Expense
      </h1>
      <p
        className="mb-8 font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Record an expense against a {entityLabels.project.toLowerCase()} budget.
      </p>

      <form
        onSubmit={handleSubmit}
        className="rounded-xl p-6"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <div className="flex flex-col gap-5">
          {/* Production selector */}
          <div>
            <label
              htmlFor="production"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {entityLabels.project} <span className="text-red-500">*</span>
            </label>
            <select
              id="production"
              value={selectedProduction}
              onChange={(e) => setSelectedProduction(e.target.value)}
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: `1px solid ${!selectedProduction ? "var(--thittam-border, #e2e8f0)" : "var(--thittam-primary, #3b82f6)"}`,
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              <option value="">Select a {entityLabels.project.toLowerCase()}...</option>
              {mockProductions.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.title}
                </option>
              ))}
            </select>
          </div>

          {/* Category dropdown */}
          <div>
            <label
              htmlFor="category"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Category <span className="text-red-500">*</span>
            </label>
            <select
              id="category"
              value={selectedCategory}
              onChange={(e) => {
                setSelectedCategory(e.target.value);
                setSelectedPo("");
              }}
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              <option value="">Select a category...</option>
              {EXPENSE_CATEGORIES.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                </option>
              ))}
            </select>

            {/* Tax treatment badge appears when category selected */}
            {category && (
              <div className="mt-2 flex items-center gap-2">
                <TaxTreatmentBadge treatment={category.taxTreatment} />
                {category.requiresPo && (
                  <span
                    className="text-[10px] font-heading font-medium"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    PO required
                  </span>
                )}
              </div>
            )}
          </div>

          {/* PO selector — shown only when category requires PO */}
          {category?.requiresPo && (
            <div>
              <label
                htmlFor="po"
                className="mb-1.5 block text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Purchase Order
              </label>
              <select
                id="po"
                value={selectedPo}
                onChange={(e) => setSelectedPo(e.target.value)}
                className="w-full rounded-lg px-3 py-2 text-sm font-mono tabular-nums"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                  color: "var(--thittam-foreground, #0f172a)",
                }}
              >
                <option value="">Select a PO...</option>
                {mockPurchaseOrders.map((po) => (
                  <option key={po.id} value={po.id}>
                    {po.number} — {po.vendor}
                  </option>
                ))}
              </select>

              {/* No PO warning */}
              {!selectedPo && (
                <div
                  className="mt-2 flex items-center gap-1.5 rounded-lg px-3 py-2"
                  style={{ backgroundColor: "#fef3c7" }}
                >
                  <AlertTriangle className="h-3.5 w-3.5" style={{ color: "#92400e" }} />
                  <span
                    className="text-xs font-body"
                    style={{ color: "#92400e" }}
                  >
                    This category requires a PO. Submitting without a PO may delay approval.
                  </span>
                </div>
              )}
            </div>
          )}

          {/* Amount */}
          <div>
            <label
              htmlFor="amount"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Amount (INR) <span className="text-red-500">*</span>
            </label>
            <input
              id="amount"
              type="text"
              inputMode="decimal"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="0.00"
              className="w-full rounded-lg px-3 py-2 text-sm font-mono tabular-nums"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Description */}
          <div>
            <label
              htmlFor="description"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Description <span className="text-red-500">*</span>
            </label>
            <textarea
              id="description"
              rows={3}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Describe the expense..."
              className="w-full resize-y rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Budget line (optional) */}
          <div>
            <label
              htmlFor="budget-line"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Budget Line (optional)
            </label>
            <select
              id="budget-line"
              value={selectedBudgetLine}
              onChange={(e) => setSelectedBudgetLine(e.target.value)}
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              <option value="">No budget line linked</option>
              {mockBudgetLines.map((bl) => (
                <option key={bl.id} value={bl.id}>
                  {bl.label}
                </option>
              ))}
            </select>
            <p
              className="mt-1 text-xs font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Link this expense to a specific budget line item for tracking.
            </p>
          </div>
        </div>

        {/* Actions */}
        <div className="mt-8 flex items-center justify-end gap-3">
          <Link
            href="/expenses"
            className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
            style={{
              color: "var(--thittam-muted-foreground, #64748b)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            Cancel
          </Link>
          <button
            type="submit"
            disabled={isSubmitting || !selectedProduction || !selectedCategory || !amount.trim() || !description.trim()}
            className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            {isSubmitting ? "Submitting..." : "Submit Expense"}
          </button>
        </div>
      </form>

      {/* Success toast */}
      {showToast && (
        <div
          className="fixed bottom-6 right-6 z-50 flex items-center gap-2 rounded-lg px-4 py-3 text-sm font-heading font-medium text-white shadow-lg"
          style={{ backgroundColor: "var(--thittam-status-on-track, #16A34A)" }}
        >
          <Check className="h-4 w-4" />
          Expense submitted successfully
        </div>
      )}
    </div>
  );
}
