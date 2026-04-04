"use client";

import { useMemo } from "react";

// ---------------------------------------------------------------------------
// BudgetHealthPie — SVG donut chart showing on-track / at-risk / over-budget.
// ---------------------------------------------------------------------------

interface BudgetHealthPieProps {
  onTrack: number;
  atRisk: number;
  overBudget: number;
}

interface Segment {
  label: string;
  count: number;
  color: string;
  textColor: string;
}

const RADIUS = 45;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

export function BudgetHealthPie({
  onTrack,
  atRisk,
  overBudget,
}: BudgetHealthPieProps) {
  const total = onTrack + atRisk + overBudget;

  const segments = useMemo<Segment[]>(
    () => [
      {
        label: "On Track",
        count: onTrack,
        color: "var(--thittam-status-on-track, #22c55e)",
        textColor: "text-green-600",
      },
      {
        label: "At Risk",
        count: atRisk,
        color: "var(--thittam-status-at-risk, #f59e0b)",
        textColor: "text-amber-600",
      },
      {
        label: "Over Budget",
        count: overBudget,
        color: "var(--thittam-status-over-budget, #ef4444)",
        textColor: "text-red-600",
      },
    ],
    [onTrack, atRisk, overBudget],
  );

  // Build stroke-dasharray / dashoffset for each arc
  const arcs = useMemo(() => {
    if (total === 0) return [];

    let offset = 0;
    return segments.map((seg) => {
      const pct = seg.count / total;
      const dash = pct * CIRCUMFERENCE;
      const gap = CIRCUMFERENCE - dash;
      const currentOffset = offset;
      offset += dash;

      return {
        ...seg,
        dashArray: `${dash} ${gap}`,
        dashOffset: -currentOffset,
      };
    });
  }, [segments, total]);

  return (
    <div
      className="rounded-lg bg-white shadow-sm p-5"
      style={{
        backgroundColor: "var(--thittam-card-bg, #ffffff)",
        color: "var(--thittam-foreground, #0f172a)",
      }}
    >
      <h3 className="text-xs font-medium uppercase tracking-wide font-heading text-gray-500 mb-4">
        Budget Health
      </h3>

      <div className="flex flex-col items-center">
        {/* Donut */}
        <div className="relative w-40 h-40">
          <svg viewBox="0 0 100 100" className="w-full h-full -rotate-90">
            {/* Background ring */}
            <circle
              cx="50"
              cy="50"
              r={RADIUS}
              fill="none"
              stroke="#e5e7eb"
              strokeWidth="8"
            />
            {/* Data arcs */}
            {arcs.map((arc) => (
              <circle
                key={arc.label}
                cx="50"
                cy="50"
                r={RADIUS}
                fill="none"
                stroke={arc.color}
                strokeWidth="8"
                strokeDasharray={arc.dashArray}
                strokeDashoffset={arc.dashOffset}
                strokeLinecap="butt"
              />
            ))}
          </svg>

          {/* Center label */}
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="text-2xl font-semibold font-mono tabular-nums">
              {total}
            </span>
            <span className="text-xs text-gray-500 font-heading">Projects</span>
          </div>
        </div>

        {/* Legend */}
        <div className="mt-4 flex gap-4 text-sm">
          {segments.map((seg) => (
            <div key={seg.label} className="flex items-center gap-1.5">
              <span
                className="inline-block h-2.5 w-2.5 rounded-full"
                style={{ backgroundColor: seg.color }}
              />
              <span className="text-gray-600 font-heading">{seg.label}</span>
              <span className={`font-mono tabular-nums font-medium ${seg.textColor}`}>
                {seg.count}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
