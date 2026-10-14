"use client";

import { useMemo, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Send,
  CheckCircle2,
  XCircle,
  X,
  ShieldCheck,
  Clock,
  FileText,
  Upload,
} from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import type { Expense as ApiExpense, ExpenseCategory } from "@/lib/api/expenses";
import {
  useApproveExpense,
  useExpense,
  useExpenseCategories,
  useRejectExpense,
} from "@/lib/hooks/use-expenses";
import { useProductions } from "@/lib/hooks/use-productions";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type ExpenseStatus = "draft" | "submitted" | "approved" | "rejected" | "paid";
type TaxTreatment = "gst_18" | "gst_12" | "gst_5" | "gst_exempt" | "tds_194c" | "tds_194j";
type POStatus = "draft" | "sent" | "acknowledged" | "approved" | "closed";

interface ApprovalEntry {
  id: string;
  action: "submitted" | "approved" | "rejected" | "paid";
  actor: string;
  role: string;
  timestamp: string;
  note: string | null;
}

interface ExpenseDetail {
  id: string;
  description: string;
  category: string;
  taxTreatment: TaxTreatment;
  amount: string;
  taxAmount: string;
  currency: string;
  status: ExpenseStatus;
  poNumber: string | null;
  poId: string | null;
  poStatus: POStatus | null;
  budgetLineRef: string | null;
  submittedBy: string;
  submittedDate: string;
  approvedBy: string | null;
  approvedDate: string | null;
  productionId: string;
  productionTitle: string;
  isPettyCash: boolean;
  approvalHistory: ApprovalEntry[];
  receipts: string[];
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
// PO Status Pipeline
// ---------------------------------------------------------------------------
const PO_STAGES: POStatus[] = ["draft", "sent", "acknowledged", "approved", "closed"];
const PO_STAGE_LABELS: Record<POStatus, string> = {
  draft: "Draft",
  sent: "Sent",
  acknowledged: "Ack",
  approved: "Approved",
  closed: "Closed",
};

function POStatusPipeline({ currentStatus }: { currentStatus: POStatus }) {
  const currentIdx = PO_STAGES.indexOf(currentStatus);
  return (
    <div className="flex items-center gap-1">
      {PO_STAGES.map((stage, idx) => (
        <div key={stage} className="flex items-center gap-1">
          <div
            className="flex h-5 items-center rounded-full px-2 text-[9px] font-heading font-medium"
            style={
              idx <= currentIdx
                ? {
                    backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 15%, transparent)",
                    color: "var(--thittam-primary, #3b82f6)",
                  }
                : {
                    backgroundColor: "var(--thittam-muted, #f1f5f9)",
                    color: "var(--thittam-muted-foreground, #64748b)",
                  }
            }
          >
            {PO_STAGE_LABELS[stage]}
          </div>
          {idx < PO_STAGES.length - 1 && (
            <div
              className="h-0.5 w-2"
              style={{
                backgroundColor:
                  idx < currentIdx
                    ? "var(--thittam-primary, #3b82f6)"
                    : "var(--thittam-border, #e2e8f0)",
              }}
            />
          )}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Approval limit / dual approval indicators
// ---------------------------------------------------------------------------
const APPROVAL_LIMIT = "500000.00"; // 5 lakhs
const DUAL_APPROVAL_THRESHOLD = "1000000.00"; // 10 lakhs

function ApprovalLimitBadge({ role, limit }: { role: string; limit: string }) {
  return (
    <div
      className="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-heading"
      style={{
        backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 8%, transparent)",
        color: "var(--thittam-primary, #3b82f6)",
      }}
    >
      <ShieldCheck className="h-3.5 w-3.5" />
      {role} limit: <span className="font-mono tabular-nums">{limit}</span>
    </div>
  );
}

function DualApprovalIndicator() {
  return (
    <div
      className="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-heading"
      style={{ backgroundColor: "#fef3c7", color: "#92400e" }}
    >
      <ShieldCheck className="h-3.5 w-3.5" />
      Dual approval required (amount exceeds threshold)
    </div>
  );
}

// ---------------------------------------------------------------------------
// API → detail mapping
// ---------------------------------------------------------------------------
const TAX_TREATMENTS: TaxTreatment[] = ["gst_18", "gst_12", "gst_5", "gst_exempt", "tds_194c", "tds_194j"];

function toDetail(
  e: ApiExpense,
  categories: Map<string, ExpenseCategory>,
  productionTitle: string,
): ExpenseDetail {
  const cat = categories.get(e.category_id);
  const tax = cat?.tax_treatment as TaxTreatment | undefined;
  const history: ApprovalEntry[] = [
    {
      id: `${e.id}-submitted`,
      action: "submitted",
      actor: e.submitted_by ?? "",
      role: "",
      timestamp: e.submitted_at ?? e.created_at ?? "",
      note: null,
    },
  ];
  if (e.approved_at && (e.status === "approved" || e.status === "rejected")) {
    history.push({
      id: `${e.id}-${e.status}`,
      action: e.status,
      actor: e.approved_by ?? "",
      role: "",
      timestamp: e.approved_at,
      note: null,
    });
  }
  return {
    id: e.id,
    description: e.description,
    category: cat?.label ?? "Uncategorised",
    taxTreatment: tax && TAX_TREATMENTS.includes(tax) ? tax : "gst_exempt",
    amount: e.amount,
    taxAmount: e.tax_amount,
    currency: e.currency,
    status: e.status as ExpenseStatus,
    poNumber: null,
    poId: e.purchase_order_id || null,
    poStatus: null,
    budgetLineRef: e.budget_line_id || null,
    submittedBy: e.submitted_by ?? "",
    submittedDate: (e.submitted_at ?? "").slice(0, 10),
    approvedBy: e.approved_by || null,
    approvedDate: e.approved_at ? e.approved_at.slice(0, 10) : null,
    productionId: e.production_id,
    productionTitle,
    isPettyCash: false,
    approvalHistory: history,
    receipts: [],
  };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function formatTimestamp(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleDateString("en-IN", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const ACTION_ICONS: Record<string, typeof CheckCircle2> = {
  submitted: Send,
  approved: CheckCircle2,
  rejected: XCircle,
  paid: CheckCircle2,
};

const ACTION_COLORS: Record<string, string> = {
  submitted: "#3b82f6",
  approved: "#16a34a",
  rejected: "#dc2626",
  paid: "#059669",
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function ExpenseDetailPage() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();

  const expenseQuery = useExpense(params.id);
  const productionsQuery = useProductions();
  const categoriesQuery = useExpenseCategories();
  const approveMutation = useApproveExpense();
  const rejectMutation = useRejectExpense();

  const expense = useMemo<ExpenseDetail | null>(() => {
    if (!expenseQuery.data) return null;
    const cats = new Map((categoriesQuery.data?.categories ?? []).map((c) => [c.id, c]));
    const title =
      (productionsQuery.data?.productions ?? []).find((p) => p.id === expenseQuery.data?.production_id)?.title ??
      expenseQuery.data.production_id;
    return toDetail(expenseQuery.data, cats, title);
  }, [expenseQuery.data, categoriesQuery.data, productionsQuery.data]);
  const [showApprovalDialog, setShowApprovalDialog] = useState(false);
  const [approvalAction, setApprovalAction] = useState<"approve" | "reject">("approve");
  const [approvalNote, setApprovalNote] = useState("");

  if (expenseQuery.isLoading) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <p className="font-body text-sm" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
          Loading expense…
        </p>
      </div>
    );
  }

  if (!expense) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <h1
          className="font-heading text-xl font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Expense not found
        </h1>
        <p
          className="mt-2 font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          The expense you are looking for does not exist or has been removed.
        </p>
        <Link
          href="/expenses"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Expenses
        </Link>
      </div>
    );
  }

  const isSubmitted = expense.status === "submitted";
  const expenseAmount = parseFloat(expense.amount);
  const needsDualApproval = expenseAmount >= parseFloat(DUAL_APPROVAL_THRESHOLD);

  const decisionPending = approveMutation.isPending || rejectMutation.isPending;
  const decisionError = (approveMutation.error ?? rejectMutation.error)?.message ?? null;

  function handleDecision() {
    const done = () => {
      setShowApprovalDialog(false);
      setApprovalNote("");
    };
    if (approvalAction === "approve") {
      approveMutation.mutate(expense!.id, { onSuccess: done });
    } else {
      rejectMutation.mutate({ id: expense!.id, reason: approvalNote.trim() }, { onSuccess: done });
    }
  }

  return (
    <div className="mx-auto max-w-5xl">
      {/* Back link */}
      <Link
        href="/expenses"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Expenses
      </Link>

      {/* Header */}
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <h1
            className="font-heading text-2xl font-semibold tracking-tight"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {expense.description}
          </h1>
          <StatusBadge status={expense.status} />
        </div>

        {/* Action buttons based on status */}
        <div className="flex items-center gap-2">
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
        </div>
      </div>

      {/* Approval limit indicators — shown for submitted expenses */}
      {isSubmitted && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <ApprovalLimitBadge role="Line Producer" limit={`INR ${parseFloat(APPROVAL_LIMIT).toLocaleString("en-IN")}`} />
          {needsDualApproval && <DualApprovalIndicator />}
        </div>
      )}

      {/* Detail card */}
      <div
        className="mb-6 rounded-xl p-5"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {/* Category */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Category
            </span>
            <div className="mt-1 flex items-center gap-2">
              <span
                className="text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {expense.category}
              </span>
              <TaxTreatmentBadge treatment={expense.taxTreatment} />
            </div>
          </div>

          {/* Amount */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Amount
            </span>
            <div className="mt-1">
              <AmountDisplay amount={expense.amount} currency={expense.currency} size="lg" />
            </div>
          </div>

          {/* Tax */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Tax Amount
            </span>
            <div className="mt-1">
              <AmountDisplay amount={expense.taxAmount} currency={expense.currency} size="sm" />
            </div>
          </div>

          {/* Production */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {entityLabels.project}
            </span>
            <Link
              href={`/productions/${expense.productionId}`}
              className="mt-1 text-sm font-body hover:underline"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            >
              {expense.productionTitle}
            </Link>
          </div>

          {/* PO reference */}
          {expense.poNumber && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Purchase Order
              </span>
              <div className="mt-1 flex flex-col gap-1">
                <Link
                  href="/expenses/purchase-orders"
                  className="font-mono tabular-nums text-sm hover:underline"
                  style={{ color: "var(--thittam-primary, #3b82f6)" }}
                >
                  {expense.poNumber}
                </Link>
                {expense.poStatus && <POStatusPipeline currentStatus={expense.poStatus} />}
              </div>
            </div>
          )}

          {/* Budget line */}
          {expense.budgetLineRef && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Budget Line
              </span>
              <span
                className="mt-1 text-sm font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {expense.budgetLineRef}
              </span>
            </div>
          )}

          {/* Submitted by */}
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Submitted By
            </span>
            <span
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {expense.submittedBy}
            </span>
            <span
              className="ml-2 font-mono tabular-nums text-xs"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {expense.submittedDate}
            </span>
          </div>

          {/* Approved by */}
          {expense.approvedBy && (
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Approved By
              </span>
              <span
                className="mt-1 text-sm font-body"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {expense.approvedBy}
              </span>
              <span
                className="ml-2 font-mono tabular-nums text-xs"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                {expense.approvedDate}
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Approval history timeline */}
      {expense.approvalHistory.length > 0 && (
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
            Approval History
          </h2>
          <div className="flex flex-col gap-0">
            {expense.approvalHistory.map((entry, idx) => {
              const Icon = ACTION_ICONS[entry.action] ?? Clock;
              const color = ACTION_COLORS[entry.action] ?? "#64748b";
              const isLast = idx === expense.approvalHistory.length - 1;

              return (
                <div key={entry.id} className="flex gap-3">
                  {/* Timeline line + dot */}
                  <div className="flex flex-col items-center">
                    <div
                      className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full"
                      style={{ backgroundColor: `${color}20`, color }}
                    >
                      <Icon className="h-3.5 w-3.5" />
                    </div>
                    {!isLast && (
                      <div
                        className="w-0.5 grow"
                        style={{ backgroundColor: "var(--thittam-border, #e2e8f0)" }}
                      />
                    )}
                  </div>

                  {/* Content */}
                  <div className={`pb-4 ${isLast ? "" : ""}`}>
                    <div className="flex items-center gap-2">
                      <span
                        className="text-sm font-heading font-medium capitalize"
                        style={{ color }}
                      >
                        {entry.action}
                      </span>
                      <span
                        className="text-xs font-body"
                        style={{ color: "var(--thittam-foreground, #0f172a)" }}
                      >
                        by {entry.actor}
                      </span>
                      <span
                        className="text-[10px] font-heading"
                        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                      >
                        ({entry.role})
                      </span>
                    </div>
                    <span
                      className="block font-mono tabular-nums text-xs"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      {formatTimestamp(entry.timestamp)}
                    </span>
                    {entry.note && (
                      <p
                        className="mt-1 text-xs font-body"
                        style={{ color: "var(--thittam-foreground, #0f172a)" }}
                      >
                        {entry.note}
                      </p>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Receipts section */}
      <div
        className="mb-6 rounded-xl p-5"
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
            Receipts & Documents ({expense.receipts.length})
          </h2>
          {isSubmitted && (
            <button
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-heading font-medium transition-colors hover:opacity-90"
              style={{
                color: "var(--thittam-primary, #3b82f6)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <Upload className="h-3.5 w-3.5" />
              Upload
            </button>
          )}
        </div>

        {expense.receipts.length > 0 ? (
          <div className="flex flex-col gap-2">
            {expense.receipts.map((receipt) => (
              <div
                key={receipt}
                className="flex items-center gap-2 rounded-lg px-3 py-2"
                style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
              >
                <FileText
                  className="h-4 w-4 shrink-0"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                />
                <span
                  className="text-sm font-mono"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {receipt}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p
            className="text-sm font-body"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            No documents uploaded yet.
          </p>
        )}
      </div>

      {/* ── Approval Dialog ─────────────────────────────────────────────── */}
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
                {approvalAction === "approve" ? "Approve Expense" : "Reject Expense"}
              </h2>
              <button
                onClick={() => setShowApprovalDialog(false)}
                className="rounded-md p-1 transition-colors hover:bg-gray-100"
              >
                <X className="h-5 w-5" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
              </button>
            </div>

            <div className="mb-3">
              <AmountDisplay amount={expense.amount} currency={expense.currency} size="lg" />
              <div className="mt-1">
                <TaxTreatmentBadge treatment={expense.taxTreatment} />
              </div>
            </div>

            {needsDualApproval && approvalAction === "approve" && (
              <div className="mb-3">
                <DualApprovalIndicator />
              </div>
            )}

            <p
              className="mb-4 font-body text-sm"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {approvalAction === "approve"
                ? `You are about to approve this expense. ${needsDualApproval ? "A second approval will be required due to the amount." : ""}`
                : "You are about to reject this expense. It will return to draft status for revision."}
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

            {decisionError && (
              <p className="mb-3 text-sm" style={{ color: "var(--thittam-status-over-budget, #DC2626)" }}>
                {decisionError}
              </p>
            )}

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
                disabled={decisionPending || (approvalAction === "reject" && !approvalNote.trim())}
                onClick={handleDecision}
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
