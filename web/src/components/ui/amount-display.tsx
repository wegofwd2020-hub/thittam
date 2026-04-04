"use client";

import { useMemo } from "react";
import { TrendingUp, TrendingDown, Minus } from "lucide-react";

// ---------------------------------------------------------------------------
// AmountDisplay — formatted monetary value with optional trend indicator.
// ---------------------------------------------------------------------------

interface AmountDisplayProps {
  amount: string | number;
  currency?: string;
  size?: "sm" | "md" | "lg";
  trend?: "up" | "down" | "neutral";
}

const SIZE_CLASSES: Record<string, string> = {
  sm: "text-sm",
  md: "text-base",
  lg: "text-xl font-semibold",
};

const TREND_ICON_SIZE: Record<string, string> = {
  sm: "h-3 w-3",
  md: "h-4 w-4",
  lg: "h-5 w-5",
};

/**
 * Formats a number using the Indian numbering system (lakhs / crores).
 * e.g. 1234567.89 -> "12,34,567.89"
 */
function formatIndian(num: number): string {
  const [intPart, decPart] = Math.abs(num).toFixed(2).split(".");
  const sign = num < 0 ? "-" : "";

  if (intPart.length <= 3) {
    return `${sign}${intPart}.${decPart}`;
  }

  // Last 3 digits, then groups of 2
  const last3 = intPart.slice(-3);
  const rest = intPart.slice(0, -3);
  const grouped = rest.replace(/\B(?=(\d{2})+(?!\d))/g, ",");

  return `${sign}${grouped},${last3}.${decPart}`;
}

function formatAmount(amount: string | number, currency?: string): string {
  const num = typeof amount === "string" ? parseFloat(amount) : amount;
  if (isNaN(num)) return String(amount);

  if (currency === "INR") {
    return `\u20B9${formatIndian(num)}`;
  }

  // Default: standard formatting with 2 decimal places
  const symbol = currency === "USD" ? "$" : currency === "EUR" ? "\u20AC" : currency === "GBP" ? "\u00A3" : currency ? `${currency} ` : "$";
  return `${symbol}${num.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function AmountDisplay({
  amount,
  currency,
  size = "md",
  trend,
}: AmountDisplayProps) {
  const formatted = useMemo(
    () => formatAmount(amount, currency),
    [amount, currency],
  );

  const trendColor =
    trend === "up"
      ? "text-green-600"
      : trend === "down"
        ? "text-red-600"
        : "text-gray-400";

  const TrendIcon =
    trend === "up"
      ? TrendingUp
      : trend === "down"
        ? TrendingDown
        : trend === "neutral"
          ? Minus
          : null;

  const iconSize = TREND_ICON_SIZE[size] ?? TREND_ICON_SIZE.md;

  return (
    <span
      className={`inline-flex items-center gap-1.5 font-mono tabular-nums ${SIZE_CLASSES[size] ?? SIZE_CLASSES.md}`}
      style={{ color: "var(--thittam-foreground, #0f172a)" }}
      data-monetary
    >
      {formatted}
      {TrendIcon && (
        <TrendIcon className={`${iconSize} ${trendColor}`} aria-hidden="true" />
      )}
    </span>
  );
}
