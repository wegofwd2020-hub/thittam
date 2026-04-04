"use client";

// ---------------------------------------------------------------------------
// PlanBadge — renders a colored pill for tenant plan tiers.
// ---------------------------------------------------------------------------

interface PlanBadgeProps {
  plan: string; // starter | professional | enterprise
}

const PLAN_STYLES: Record<string, { bg: string; text: string; label: string }> = {
  starter: { bg: "bg-gray-100", text: "text-gray-700", label: "Starter" },
  professional: { bg: "bg-blue-100", text: "text-blue-700", label: "Professional" },
  enterprise: { bg: "bg-purple-100", text: "text-purple-700", label: "Enterprise" },
};

const FALLBACK = { bg: "bg-gray-100", text: "text-gray-600", label: "Unknown" };

export function PlanBadge({ plan }: PlanBadgeProps) {
  const style = PLAN_STYLES[plan.toLowerCase()] ?? FALLBACK;

  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium font-heading ${style.bg} ${style.text}`}
    >
      {style.label}
    </span>
  );
}
