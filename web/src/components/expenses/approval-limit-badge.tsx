"use client";

// ---------------------------------------------------------------------------
// ApprovalLimitBadge — shows whether an expense is within a role's limit.
// ---------------------------------------------------------------------------

interface ApprovalLimitBadgeProps {
  role: string;
  maxAmount: string;
  expenseAmount: string;
  currency?: string;
}

function parseDecimal(value: string): number {
  const num = parseFloat(value);
  return isNaN(num) ? 0 : num;
}

function formatAmount(num: number, currency: string): string {
  return `${currency}${num.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function ApprovalLimitBadge({
  role,
  maxAmount,
  expenseAmount,
  currency = "$",
}: ApprovalLimitBadgeProps) {
  const limit = parseDecimal(maxAmount);
  const expense = parseDecimal(expenseAmount);
  const withinLimit = expense <= limit;

  return (
    <div
      className="inline-flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs"
      style={{
        borderColor: withinLimit
          ? "rgb(134 239 172)" // green-300
          : "rgb(252 165 165)", // red-300
        backgroundColor: withinLimit
          ? "rgb(240 253 244)" // green-50
          : "rgb(254 242 242)", // red-50
      }}
    >
      <span
        className="font-heading"
        style={{
          color: withinLimit
            ? "rgb(22 101 52)" // green-800
            : "rgb(153 27 27)", // red-800
        }}
      >
        {role}
      </span>
      <span
        className="font-mono tabular-nums"
        style={{
          color: withinLimit
            ? "rgb(22 101 52)" // green-800
            : "rgb(153 27 27)", // red-800
        }}
      >
        {formatAmount(limit, currency)}
      </span>
      <span
        className="font-body"
        style={{
          color: withinLimit
            ? "rgb(21 128 61)" // green-700
            : "rgb(185 28 28)", // red-700
        }}
      >
        {withinLimit ? "Within limit" : "Exceeds limit"}
      </span>
    </div>
  );
}
