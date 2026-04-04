"use client";

import { useMemo } from "react";
import { StatusBadge } from "@/components/ui/status-badge";

// ---------------------------------------------------------------------------
// BudgetSummaryCard — budget health overview with progress bar.
// ---------------------------------------------------------------------------

interface BudgetSummaryCardProps {
  totalBudgeted: string;
  totalActual: string;
  totalCommitted: string;
  status: string;
  currency?: string;
}

function parseDecimal(value: string): number {
  const num = parseFloat(value);
  return isNaN(num) ? 0 : num;
}

function formatAmount(num: number, currency?: string): string {
  const symbol =
    currency === "INR"
      ? "\u20B9"
      : currency === "USD"
        ? "$"
        : currency === "EUR"
          ? "\u20AC"
          : currency === "GBP"
            ? "\u00A3"
            : currency
              ? `${currency} `
              : "$";

  return `${symbol}${num.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function BudgetSummaryCard({
  totalBudgeted,
  totalActual,
  totalCommitted,
  status,
  currency,
}: BudgetSummaryCardProps) {
  const budgeted = useMemo(() => parseDecimal(totalBudgeted), [totalBudgeted]);
  const actual = useMemo(() => parseDecimal(totalActual), [totalActual]);
  const committed = useMemo(
    () => parseDecimal(totalCommitted),
    [totalCommitted],
  );
  const available = budgeted - actual - committed;
  const consumedPct = budgeted > 0 ? (actual / budgeted) * 100 : 0;

  const barColor =
    consumedPct > 100
      ? "bg-red-500"
      : consumedPct >= 80
        ? "bg-amber-500"
        : "bg-green-500";

  const barWidth = Math.min(consumedPct, 100);

  return (
    <div
      className="relative rounded-lg border p-6"
      style={{
        borderColor: "var(--thittam-border, #e2e8f0)",
        backgroundColor: "var(--thittam-card, #ffffff)",
      }}
    >
      {/* Status badge — top right */}
      <div className="absolute right-4 top-4">
        <StatusBadge status={status} variant="budget" />
      </div>

      <h3
        className="mb-4 text-sm font-semibold uppercase tracking-wider font-heading"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Budget Summary
      </h3>

      {/* Amounts grid */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <div>
          <p
            className="text-xs font-heading"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Budgeted
          </p>
          <p
            className="mt-1 text-lg font-semibold font-mono tabular-nums"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {formatAmount(budgeted, currency)}
          </p>
        </div>
        <div>
          <p
            className="text-xs font-heading"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Actual
          </p>
          <p
            className="mt-1 text-lg font-semibold font-mono tabular-nums"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {formatAmount(actual, currency)}
          </p>
        </div>
        <div>
          <p
            className="text-xs font-heading"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Committed
          </p>
          <p
            className="mt-1 text-lg font-semibold font-mono tabular-nums"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {formatAmount(committed, currency)}
          </p>
        </div>
        <div>
          <p
            className="text-xs font-heading"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Available
          </p>
          <p
            className={`mt-1 text-lg font-semibold font-mono tabular-nums ${
              available < 0 ? "text-red-600" : "text-green-600"
            }`}
          >
            {formatAmount(available, currency)}
          </p>
        </div>
      </div>

      {/* Progress bar */}
      <div className="mt-5">
        <div className="mb-1 flex items-center justify-between text-xs font-heading">
          <span style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
            Consumed
          </span>
          <span
            className="font-mono tabular-nums"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {consumedPct.toFixed(1)}%
          </span>
        </div>
        <div
          className="h-2 w-full overflow-hidden rounded-full"
          style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
        >
          <div
            className={`h-full rounded-full transition-all ${barColor}`}
            style={{ width: `${barWidth}%` }}
          />
        </div>
      </div>
    </div>
  );
}
