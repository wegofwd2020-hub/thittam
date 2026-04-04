"use client";

import type { ThemeColors } from "@/lib/themes/types";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ThemePreviewPanelProps {
  colors: ThemeColors;
}

// ---------------------------------------------------------------------------
// Component — Live miniature preview of the current theme
// ---------------------------------------------------------------------------

export function ThemePreviewPanel({ colors }: ThemePreviewPanelProps) {
  return (
    <div
      className="sticky top-6 flex flex-col gap-3 rounded-xl border p-4"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        borderColor: "var(--thittam-border, #e2e8f0)",
      }}
    >
      <span
        className="text-xs font-heading font-semibold"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        Live Preview
      </span>

      {/* Mini app shell */}
      <div
        className="overflow-hidden rounded-lg border"
        style={{
          borderColor: colors.border,
          backgroundColor: colors.muted,
          minHeight: 280,
        }}
      >
        <div className="flex h-[280px]">
          {/* ── Mini Sidebar ────────────────────────────────────── */}
          <div
            className="flex w-14 flex-shrink-0 flex-col items-center gap-2 px-1 py-3 transition-colors duration-200"
            style={{ backgroundColor: colors.sidebar.bg }}
          >
            {/* Active nav item */}
            <div
              className="flex h-8 w-10 items-center justify-center rounded-md transition-colors duration-200"
              style={{ backgroundColor: colors.sidebar.activeBg }}
            >
              <div
                className="h-3.5 w-3.5 rounded-sm transition-colors duration-200"
                style={{ backgroundColor: colors.sidebar.activeItem }}
              />
            </div>
            {/* Inactive nav items */}
            {[1, 2, 3, 4].map((i) => (
              <div
                key={i}
                className="flex h-8 w-10 items-center justify-center"
              >
                <div
                  className="h-3 w-3 rounded-sm opacity-50 transition-colors duration-200"
                  style={{ backgroundColor: colors.sidebar.text }}
                />
              </div>
            ))}
          </div>

          {/* ── Mini Content Area ───────────────────────────────── */}
          <div
            className="flex flex-1 flex-col gap-2.5 p-3 transition-colors duration-200"
            style={{ backgroundColor: colors.background }}
          >
            {/* Page title bar */}
            <div className="flex items-center gap-2">
              <div
                className="h-2 w-16 rounded transition-colors duration-200"
                style={{ backgroundColor: colors.foreground, opacity: 0.8 }}
              />
            </div>

            {/* KPI Cards row */}
            <div className="flex gap-2">
              {/* KPI 1 — primary */}
              <div
                className="flex flex-1 flex-col gap-1 rounded-md p-2 transition-colors duration-200"
                style={{ backgroundColor: colors.primary }}
              >
                <div
                  className="h-1.5 w-10 rounded opacity-70"
                  style={{ backgroundColor: colors.primaryForeground }}
                />
                <div
                  className="font-mono tabular-nums text-[11px] font-bold transition-colors duration-200"
                  style={{ color: colors.primaryForeground }}
                >
                  12,50,000
                </div>
              </div>
              {/* KPI 2 — accent */}
              <div
                className="flex flex-1 flex-col gap-1 rounded-md p-2 transition-colors duration-200"
                style={{ backgroundColor: colors.accent }}
              >
                <div
                  className="h-1.5 w-10 rounded opacity-70"
                  style={{ backgroundColor: colors.accentForeground }}
                />
                <div
                  className="font-mono tabular-nums text-[11px] font-bold transition-colors duration-200"
                  style={{ color: colors.accentForeground }}
                >
                  8,75,000
                </div>
              </div>
            </div>

            {/* Status badges */}
            <div className="flex gap-1.5">
              <span
                className="rounded-full px-2 py-0.5 text-[9px] font-heading font-semibold text-white transition-colors duration-200"
                style={{ backgroundColor: colors.status.onTrack }}
              >
                On Track
              </span>
              <span
                className="rounded-full px-2 py-0.5 text-[9px] font-heading font-semibold text-white transition-colors duration-200"
                style={{ backgroundColor: colors.status.atRisk }}
              >
                At Risk
              </span>
              <span
                className="rounded-full px-2 py-0.5 text-[9px] font-heading font-semibold text-white transition-colors duration-200"
                style={{ backgroundColor: colors.status.overBudget }}
              >
                Over Budget
              </span>
            </div>

            {/* Mini table */}
            <div
              className="flex-1 rounded-md border transition-colors duration-200"
              style={{ borderColor: colors.border }}
            >
              {/* Table header */}
              <div
                className="flex items-center gap-2 border-b px-2 py-1.5 transition-colors duration-200"
                style={{
                  borderColor: colors.border,
                  backgroundColor: colors.muted,
                }}
              >
                <div
                  className="h-1.5 w-12 rounded font-heading transition-colors duration-200"
                  style={{ backgroundColor: colors.foreground, opacity: 0.6 }}
                />
                <div
                  className="h-1.5 w-8 rounded font-heading transition-colors duration-200"
                  style={{ backgroundColor: colors.foreground, opacity: 0.6 }}
                />
                <div
                  className="h-1.5 w-10 rounded font-heading transition-colors duration-200"
                  style={{ backgroundColor: colors.foreground, opacity: 0.6 }}
                />
              </div>
              {/* Table rows */}
              {[0, 1, 2].map((i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 border-b px-2 py-1.5 last:border-b-0 transition-colors duration-200"
                  style={{ borderColor: colors.border }}
                >
                  <div
                    className="h-1.5 w-12 rounded transition-colors duration-200"
                    style={{
                      backgroundColor: colors.mutedForeground,
                      opacity: 0.4,
                    }}
                  />
                  <div
                    className="font-mono tabular-nums text-[9px] transition-colors duration-200"
                    style={{ color: colors.mutedForeground }}
                  >
                    {["5,00,000", "3,25,000", "1,50,000"][i]}
                  </div>
                  <div
                    className="ml-auto h-3 w-3 rounded-full transition-colors duration-200"
                    style={{
                      backgroundColor: [
                        colors.status.onTrack,
                        colors.status.atRisk,
                        colors.status.overBudget,
                      ][i],
                    }}
                  />
                </div>
              ))}
            </div>

            {/* Sample button */}
            <button
              type="button"
              className="self-start rounded-md px-3 py-1 text-[10px] font-heading font-semibold transition-colors duration-200"
              style={{
                backgroundColor: colors.primary,
                color: colors.primaryForeground,
              }}
            >
              Sample Button
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
