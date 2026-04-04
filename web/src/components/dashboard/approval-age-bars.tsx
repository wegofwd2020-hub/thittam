"use client";

import { useMemo } from "react";

// ---------------------------------------------------------------------------
// ApprovalAgeBars — horizontal stacked bar showing approval age distribution.
// ---------------------------------------------------------------------------

interface ApprovalAgeBarsProps {
  under24h: number;
  oneToThreeDays: number;
  threeToSevenDays: number;
  overSevenDays: number;
}

interface Band {
  label: string;
  count: number;
  color: string;
  textColor: string;
}

export function ApprovalAgeBars({
  under24h,
  oneToThreeDays,
  threeToSevenDays,
  overSevenDays,
}: ApprovalAgeBarsProps) {
  const total = under24h + oneToThreeDays + threeToSevenDays + overSevenDays;

  const bands = useMemo<Band[]>(
    () => [
      {
        label: "< 24h",
        count: under24h,
        color: "var(--thittam-status-on-track, #22c55e)",
        textColor: "text-green-600",
      },
      {
        label: "1-3 days",
        count: oneToThreeDays,
        color: "var(--thittam-status-at-risk, #f59e0b)",
        textColor: "text-amber-600",
      },
      {
        label: "3-7 days",
        count: threeToSevenDays,
        color: "#f97316",
        textColor: "text-orange-600",
      },
      {
        label: "> 7 days",
        count: overSevenDays,
        color: "var(--thittam-status-over-budget, #ef4444)",
        textColor: "text-red-600",
      },
    ],
    [under24h, oneToThreeDays, threeToSevenDays, overSevenDays],
  );

  const nonZeroBands = bands.filter((b) => b.count > 0);

  return (
    <div
      className="rounded-lg bg-white shadow-sm p-5"
      style={{
        backgroundColor: "var(--thittam-card-bg, #ffffff)",
        color: "var(--thittam-foreground, #0f172a)",
      }}
    >
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-xs font-medium uppercase tracking-wide font-heading text-gray-500">
          Approval Pipeline
        </h3>
        <span className="text-sm font-mono tabular-nums font-medium">
          {total} pending
        </span>
      </div>

      {/* Stacked bar */}
      <div className="flex h-7 w-full rounded overflow-hidden bg-gray-100">
        {total > 0 &&
          nonZeroBands.map((band) => {
            const pct = (band.count / total) * 100;
            return (
              <div
                key={band.label}
                className="flex items-center justify-center text-xs font-mono tabular-nums text-white font-medium transition-all"
                style={{
                  width: `${pct}%`,
                  backgroundColor: band.color,
                  minWidth: pct > 0 ? "24px" : 0,
                }}
                title={`${band.label}: ${band.count}`}
              >
                {pct >= 10 && band.count}
              </div>
            );
          })}
      </div>

      {/* Labels */}
      <div className="mt-3 grid grid-cols-4 gap-2 text-center">
        {bands.map((band) => (
          <div key={band.label}>
            <div className={`text-sm font-mono tabular-nums font-medium ${band.textColor}`}>
              {band.count}
            </div>
            <div className="text-xs text-gray-500 font-heading">{band.label}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
