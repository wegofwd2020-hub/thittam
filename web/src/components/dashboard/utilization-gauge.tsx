"use client";

import { useMemo } from "react";

// ---------------------------------------------------------------------------
// UtilizationGauge — SVG circular progress gauge with percentage at center.
// ---------------------------------------------------------------------------

interface UtilizationGaugeProps {
  percentage: number;
  label: string;
  assigned: number;
  total: number;
}

const RADIUS = 42;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

function getGaugeColor(pct: number): string {
  if (pct >= 80) return "var(--thittam-status-on-track, #22c55e)";
  if (pct >= 50) return "var(--thittam-status-at-risk, #f59e0b)";
  return "var(--thittam-status-over-budget, #ef4444)";
}

export function UtilizationGauge({
  percentage,
  label,
  assigned,
  total,
}: UtilizationGaugeProps) {
  const clamped = useMemo(
    () => Math.max(0, Math.min(100, percentage)),
    [percentage],
  );

  const strokeDash = (clamped / 100) * CIRCUMFERENCE;
  const color = getGaugeColor(clamped);

  return (
    <div
      className="rounded-lg bg-white shadow-sm p-5 flex flex-col items-center"
      style={{
        backgroundColor: "var(--thittam-card-bg, #ffffff)",
        color: "var(--thittam-foreground, #0f172a)",
      }}
    >
      {/* SVG gauge */}
      <div className="relative w-32 h-32">
        <svg viewBox="0 0 100 100" className="w-full h-full -rotate-90">
          {/* Background track */}
          <circle
            cx="50"
            cy="50"
            r={RADIUS}
            fill="none"
            stroke="#e5e7eb"
            strokeWidth="7"
          />
          {/* Progress arc */}
          <circle
            cx="50"
            cy="50"
            r={RADIUS}
            fill="none"
            stroke={color}
            strokeWidth="7"
            strokeDasharray={`${strokeDash} ${CIRCUMFERENCE - strokeDash}`}
            strokeDashoffset="0"
            strokeLinecap="round"
          />
        </svg>

        {/* Center percentage */}
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-xl font-semibold font-mono tabular-nums">
            {clamped}%
          </span>
        </div>
      </div>

      {/* Label */}
      <span className="mt-2 text-sm font-medium font-heading text-gray-600">
        {label}
      </span>

      {/* Assigned / Total */}
      <span className="mt-1 text-xs text-gray-400 font-mono tabular-nums">
        {assigned} / {total}
      </span>
    </div>
  );
}
