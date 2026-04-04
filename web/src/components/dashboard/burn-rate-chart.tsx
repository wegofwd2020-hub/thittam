"use client";

import { useMemo } from "react";

// ---------------------------------------------------------------------------
// BurnRateChart — horizontal bar chart comparing actual vs budget per month.
// ---------------------------------------------------------------------------

interface BurnRateChartProps {
  data: { month: string; actual: number; budget: number }[];
}

export function BurnRateChart({ data }: BurnRateChartProps) {
  const maxValue = useMemo(
    () =>
      data.reduce((max, d) => Math.max(max, d.actual, d.budget), 0) || 1,
    [data],
  );

  return (
    <div
      className="rounded-lg bg-white shadow-sm p-5"
      style={{
        backgroundColor: "var(--thittam-card-bg, #ffffff)",
        color: "var(--thittam-foreground, #0f172a)",
      }}
    >
      <h3 className="text-xs font-medium uppercase tracking-wide font-heading text-gray-500 mb-4">
        Monthly Burn Rate
      </h3>

      <div className="space-y-3">
        {data.map((d) => {
          const budgetPct = (d.budget / maxValue) * 100;
          const actualPct = (d.actual / maxValue) * 100;
          const overBudget = d.actual > d.budget;

          return (
            <div key={d.month}>
              {/* Month label + amounts */}
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium font-heading text-gray-600">
                  {d.month}
                </span>
                <div className="flex items-center gap-3 text-xs font-mono tabular-nums">
                  <span className="text-gray-400">
                    Budget: {formatCompact(d.budget)}
                  </span>
                  <span className={overBudget ? "text-red-600" : "text-gray-700"}>
                    Actual: {formatCompact(d.actual)}
                  </span>
                </div>
              </div>

              {/* Bars */}
              <div className="relative h-5 rounded bg-gray-100">
                {/* Budget bar (muted) */}
                <div
                  className="absolute inset-y-0 left-0 rounded bg-gray-200"
                  style={{ width: `${budgetPct}%` }}
                />
                {/* Actual bar */}
                <div
                  className="absolute inset-y-0 left-0 rounded"
                  style={{
                    width: `${actualPct}%`,
                    backgroundColor: overBudget
                      ? "var(--thittam-status-over-budget, #ef4444)"
                      : "var(--thittam-primary, #3b82f6)",
                    opacity: 0.85,
                  }}
                />
              </div>
            </div>
          );
        })}
      </div>

      {/* Legend */}
      <div className="mt-4 flex gap-4 text-xs text-gray-500">
        <div className="flex items-center gap-1.5">
          <span className="inline-block h-2.5 w-2.5 rounded bg-gray-200" />
          <span className="font-heading">Budget</span>
        </div>
        <div className="flex items-center gap-1.5">
          <span
            className="inline-block h-2.5 w-2.5 rounded"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          />
          <span className="font-heading">Actual</span>
        </div>
      </div>
    </div>
  );
}

/** Compact number formatter: 1234567 -> "1.23M", 45000 -> "45K" */
function formatCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`;
  return n.toLocaleString();
}
