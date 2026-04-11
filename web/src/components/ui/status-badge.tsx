"use client";

import { useMemo } from "react";

// ---------------------------------------------------------------------------
// StatusBadge — renders a small pill with status-appropriate color.
// ---------------------------------------------------------------------------

interface StatusBadgeProps {
  status: string;
  variant?: "default" | "phase" | "budget" | "health";
}

interface BadgeStyle {
  bg: string;
  text: string;
}

const STATUS_COLORS: Record<string, BadgeStyle> = {
  // Gray — early / inactive states
  development: { bg: "bg-gray-100", text: "text-gray-700" },
  draft: { bg: "bg-gray-100", text: "text-gray-700" },
  pending: { bg: "bg-gray-100", text: "text-gray-700" },

  // Blue — planning states
  pre_production: { bg: "bg-blue-100", text: "text-blue-700" },
  submitted: { bg: "bg-blue-100", text: "text-blue-700" },
  planning: { bg: "bg-blue-100", text: "text-blue-700" },

  // Green — active / approved states
  production: { bg: "bg-green-100", text: "text-green-700" },
  active: { bg: "bg-green-100", text: "text-green-700" },
  approved: { bg: "bg-green-100", text: "text-green-700" },

  // Purple — locked / post states
  post_production: { bg: "bg-purple-100", text: "text-purple-700" },
  locked: { bg: "bg-purple-100", text: "text-purple-700" },

  // Emerald — completed / final states
  released: { bg: "bg-emerald-100", text: "text-emerald-700" },
  completed: { bg: "bg-emerald-100", text: "text-emerald-700" },
  paid: { bg: "bg-emerald-100", text: "text-emerald-700" },
  signed: { bg: "bg-emerald-100", text: "text-emerald-700" },

  // Blue — awaiting action
  pending_signature: { bg: "bg-blue-100", text: "text-blue-700" },

  // Slate — archived
  archived: { bg: "bg-slate-100", text: "text-slate-600" },

  // Amber — on hold
  on_hold: { bg: "bg-amber-100", text: "text-amber-700" },

  // Red — cancelled / rejected
  cancelled: { bg: "bg-red-100", text: "text-red-700" },
  rejected: { bg: "bg-red-100", text: "text-red-700" },
};

const HEALTH_COLORS: Record<string, BadgeStyle> = {
  on_track: { bg: "bg-green-100", text: "text-green-700" },
  at_risk: { bg: "bg-amber-100", text: "text-amber-700" },
  over_budget: { bg: "bg-red-100", text: "text-red-700" },
};

const FALLBACK: BadgeStyle = { bg: "bg-gray-100", text: "text-gray-600" };

function formatLabel(status: string): string {
  return status
    .replace(/_/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export function StatusBadge({ status, variant = "default" }: StatusBadgeProps) {
  const style = useMemo(() => {
    const key = status.toLowerCase();
    if (variant === "health") {
      return HEALTH_COLORS[key] ?? STATUS_COLORS[key] ?? FALLBACK;
    }
    return STATUS_COLORS[key] ?? FALLBACK;
  }, [status, variant]);

  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium font-heading ${style.bg} ${style.text}`}
    >
      {formatLabel(status)}
    </span>
  );
}
