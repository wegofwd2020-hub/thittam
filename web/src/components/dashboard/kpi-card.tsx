"use client";

import { useMemo } from "react";
import { TrendingUp, TrendingDown, Minus } from "lucide-react";

// ---------------------------------------------------------------------------
// KPICard — large metric card with trend indicator and colored border.
// ---------------------------------------------------------------------------

interface KPICardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  trend?: { direction: "up" | "down" | "neutral"; value: string };
  icon?: React.ReactNode;
  variant?: "default" | "success" | "warning" | "danger";
}

const BORDER_COLORS: Record<string, string> = {
  default: "border-l-[var(--thittam-primary,#3b82f6)]",
  success: "border-l-green-500",
  warning: "border-l-amber-500",
  danger: "border-l-red-500",
};

const TREND_COLORS: Record<string, string> = {
  up: "text-green-600",
  down: "text-red-600",
  neutral: "text-gray-400",
};

export function KPICard({
  title,
  value,
  subtitle,
  trend,
  icon,
  variant = "default",
}: KPICardProps) {
  const isNumeric = useMemo(
    () => typeof value === "number" || /^[\d$.,\u20B9\u20AC\u00A3%+-]+$/.test(String(value)),
    [value],
  );

  const TrendIcon =
    trend?.direction === "up"
      ? TrendingUp
      : trend?.direction === "down"
        ? TrendingDown
        : trend?.direction === "neutral"
          ? Minus
          : null;

  return (
    <div
      className={`rounded-lg border-l-4 bg-white shadow-sm p-5 ${BORDER_COLORS[variant] ?? BORDER_COLORS.default}`}
      style={{
        backgroundColor: "var(--thittam-card-bg, #ffffff)",
        color: "var(--thittam-foreground, #0f172a)",
      }}
    >
      {/* Header row: title + icon */}
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium uppercase tracking-wide font-heading text-gray-500">
          {title}
        </span>
        {icon && (
          <span className="text-gray-400" aria-hidden="true">
            {icon}
          </span>
        )}
      </div>

      {/* Value */}
      <div
        className={`text-2xl font-semibold leading-tight ${isNumeric ? "font-mono tabular-nums" : "font-heading"}`}
      >
        {typeof value === "number" ? value.toLocaleString() : value}
      </div>

      {/* Subtitle + Trend */}
      <div className="mt-2 flex items-center gap-3 text-sm">
        {subtitle && <span className="text-gray-500">{subtitle}</span>}
        {trend && TrendIcon && (
          <span
            className={`inline-flex items-center gap-1 ${TREND_COLORS[trend.direction] ?? TREND_COLORS.neutral}`}
          >
            <TrendIcon className="h-3.5 w-3.5" aria-hidden="true" />
            <span className="font-mono text-xs tabular-nums">{trend.value}</span>
          </span>
        )}
      </div>
    </div>
  );
}
