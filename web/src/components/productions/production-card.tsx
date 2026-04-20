"use client";

import Link from "next/link";
import { StatusBadge } from "@/components/ui/status-badge";

export interface Production {
  id: string;
  title: string;
  status: string;
  description: string;
  genre: string;
  // currentPhase and budgetHealth are derived from phases + budget actuals.
  // Neither is included in the Production API response; callers compute them
  // when available. Undefined means "not yet computed" — the card hides the
  // dot rather than rendering an unknown-health placeholder.
  currentPhase?: string;
  budgetHealth?: "on_track" | "at_risk" | "over_budget";
  startDate?: string;
  endDate?: string;
}

interface ProductionCardProps {
  production: Production;
}

const healthDotColor: Record<string, string> = {
  on_track: "#16A34A",
  at_risk: "#F59E0B",
  over_budget: "#DC2626",
};

const healthLabel: Record<string, string> = {
  on_track: "On Track",
  at_risk: "At Risk",
  over_budget: "Over Budget",
};

export function ProductionCard({ production }: ProductionCardProps) {
  return (
    <Link
      href={`/productions/${production.id}`}
      className="group flex flex-col gap-3 rounded-xl p-5 transition-shadow hover:shadow-md"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        border: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      {/* Header row */}
      <div className="flex items-start justify-between gap-2">
        <h3
          className="font-heading text-sm font-semibold leading-tight"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {production.title}
        </h3>
        <StatusBadge status={production.status} />
      </div>

      {/* Description */}
      <p
        className="font-body text-xs leading-relaxed line-clamp-2"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        {production.description}
      </p>

      {/* Footer row */}
      <div className="flex items-center justify-between">
        <span
          className="inline-flex items-center rounded-md px-2 py-0.5 text-[11px] font-medium font-heading"
          style={{
            backgroundColor:
              "color-mix(in srgb, var(--thittam-primary, #3b82f6) 10%, transparent)",
            color: "var(--thittam-primary, #3b82f6)",
          }}
        >
          {production.genre}
        </span>

        {production.budgetHealth && (
          <div className="flex items-center gap-1.5">
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{
                backgroundColor:
                  healthDotColor[production.budgetHealth] ?? "#94a3b8",
              }}
            />
            <span
              className="text-[11px] font-heading"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {healthLabel[production.budgetHealth] ?? "Unknown"}
            </span>
          </div>
        )}
      </div>
    </Link>
  );
}
