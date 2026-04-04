"use client";

import { useCallback, useState } from "react";
import type { Budget } from "@/lib/api/budgets";

// ---------------------------------------------------------------------------
// ApprovalDialog — modal for budget approval / rejection workflow.
// ---------------------------------------------------------------------------

interface ApprovalDialogProps {
  budget: Budget;
  totalAmount: string;
  onApprove: () => void;
  onReject: (reason: string) => void;
  isOpen: boolean;
  onClose: () => void;
}

const LARGE_BUDGET_THRESHOLD = 500000;

function parseDecimal(value: string): number {
  const num = parseFloat(value);
  return isNaN(num) ? 0 : num;
}

function formatAmount(num: number): string {
  return `$${num.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function ApprovalDialog({
  budget,
  totalAmount,
  onApprove,
  onReject,
  isOpen,
  onClose,
}: ApprovalDialogProps) {
  const [rejectReason, setRejectReason] = useState("");
  const [showRejectForm, setShowRejectForm] = useState(false);

  const amount = parseDecimal(totalAmount);
  const isLargeBudget = amount >= LARGE_BUDGET_THRESHOLD;

  const handleReject = useCallback(() => {
    if (rejectReason.trim()) {
      onReject(rejectReason.trim());
      setRejectReason("");
      setShowRejectForm(false);
    }
  }, [rejectReason, onReject]);

  const handleClose = useCallback(() => {
    setRejectReason("");
    setShowRejectForm(false);
    onClose();
  }, [onClose]);

  if (!isOpen) return null;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-40 bg-black/50"
        onClick={handleClose}
        aria-hidden="true"
      />

      {/* Dialog */}
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div
          className="w-full max-w-md rounded-lg border shadow-lg"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            backgroundColor: "var(--thittam-card, #ffffff)",
          }}
          role="dialog"
          aria-modal="true"
          aria-labelledby="approval-dialog-title"
        >
          {/* Header */}
          <div
            className="border-b px-6 py-4"
            style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
          >
            <h2
              id="approval-dialog-title"
              className="text-lg font-semibold font-heading"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Budget Approval
            </h2>
          </div>

          {/* Body */}
          <div className="px-6 py-5 space-y-4">
            <div>
              <p
                className="text-sm font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Budget
              </p>
              <p
                className="mt-0.5 text-base font-body"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {budget.label}
              </p>
            </div>

            <div>
              <p
                className="text-sm font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Total Amount
              </p>
              <p
                className="mt-0.5 text-2xl font-bold font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {formatAmount(amount)}
              </p>
            </div>

            {isLargeBudget && (
              <div
                className="rounded-md border px-3 py-2 text-xs font-body"
                style={{
                  backgroundColor: "rgb(254 252 232)", // amber-50
                  borderColor: "rgb(253 230 138)", // amber-200
                  color: "rgb(146 64 14)", // amber-800
                }}
              >
                This budget exceeds{" "}
                <span className="font-mono tabular-nums">
                  {formatAmount(LARGE_BUDGET_THRESHOLD)}
                </span>
                . Dual approval may be required per company policy.
              </div>
            )}

            {/* Reject form */}
            {showRejectForm && (
              <div className="space-y-2">
                <label
                  htmlFor="reject-reason"
                  className="block text-sm font-heading"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Rejection Reason
                </label>
                <textarea
                  id="reject-reason"
                  value={rejectReason}
                  onChange={(e) => setRejectReason(e.target.value)}
                  placeholder="Explain why this budget is being rejected..."
                  rows={3}
                  className="w-full rounded-md border px-3 py-2 text-sm font-body placeholder:text-gray-400 focus:outline-none focus:ring-2"
                  style={{
                    borderColor: "var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                    backgroundColor: "var(--thittam-card, #ffffff)",
                  }}
                />
              </div>
            )}
          </div>

          {/* Footer */}
          <div
            className="flex items-center justify-end gap-3 border-t px-6 py-4"
            style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
          >
            <button
              type="button"
              onClick={handleClose}
              className="rounded-md border px-4 py-2 text-sm font-heading transition-colors hover:bg-gray-50"
              style={{
                borderColor: "var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              Cancel
            </button>

            {showRejectForm ? (
              <button
                type="button"
                onClick={handleReject}
                disabled={!rejectReason.trim()}
                className="rounded-md bg-red-600 px-4 py-2 text-sm font-heading text-white transition-colors hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Confirm Rejection
              </button>
            ) : (
              <>
                <button
                  type="button"
                  onClick={() => setShowRejectForm(true)}
                  className="rounded-md bg-red-600 px-4 py-2 text-sm font-heading text-white transition-colors hover:bg-red-700"
                >
                  Reject
                </button>
                <button
                  type="button"
                  onClick={onApprove}
                  className="rounded-md bg-green-600 px-4 py-2 text-sm font-heading text-white transition-colors hover:bg-green-700"
                >
                  Approve
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </>
  );
}
