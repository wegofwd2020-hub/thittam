"use client";

import { useMemo } from "react";
import { TrendingUp, TrendingDown } from "lucide-react";

// ---------------------------------------------------------------------------
// VarianceIndicator — inline variance display with color-coded arrow.
// ---------------------------------------------------------------------------

interface VarianceIndicatorProps {
  budgeted: string;
  actual: string;
  committed?: string;
}

function parseDecimal(value: string): number {
  const num = parseFloat(value);
  return isNaN(num) ? 0 : num;
}

export function VarianceIndicator({
  budgeted,
  actual,
  committed,
}: VarianceIndicatorProps) {
  const variance = useMemo(() => {
    const b = parseDecimal(budgeted);
    const a = parseDecimal(actual);
    const c = committed ? parseDecimal(committed) : 0;
    return b - a - c;
  }, [budgeted, actual, committed]);

  const consumedPct = useMemo(() => {
    const b = parseDecimal(budgeted);
    if (b <= 0) return 0;
    const a = parseDecimal(actual);
    const c = committed ? parseDecimal(committed) : 0;
    return ((a + c) / b) * 100;
  }, [budgeted, actual, committed]);

  // Color logic: green = under budget, amber = >80% consumed, red = over budget
  const isOver = variance < 0;
  const isWarning = !isOver && consumedPct >= 80;

  const colorClass = isOver
    ? "text-red-600"
    : isWarning
      ? "text-amber-600"
      : "text-green-600";

  const Icon = isOver ? TrendingDown : TrendingUp;
  const sign = isOver ? "-" : "+";
  const absVariance = Math.abs(variance);

  return (
    <span
      className={`inline-flex items-center gap-1 font-mono tabular-nums text-sm ${colorClass}`}
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
      <span>
        {sign}$
        {absVariance.toLocaleString("en-US", {
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        })}
      </span>
    </span>
  );
}
