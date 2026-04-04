import type { ThittamTheme } from "./types";

/**
 * Converts a ThittamTheme into a flat Record of CSS custom property
 * names to their values.  The provider sets these on :root so that
 * Tailwind / plain CSS can reference them via var(--thittam-*).
 */
export function themeToCssVars(theme: ThittamTheme): Record<string, string> {
  const { colors } = theme;

  return {
    // Core palette
    "--thittam-primary": colors.primary,
    "--thittam-primary-foreground": colors.primaryForeground,
    "--thittam-secondary": colors.secondary,
    "--thittam-secondary-foreground": colors.secondaryForeground,
    "--thittam-accent": colors.accent,
    "--thittam-accent-foreground": colors.accentForeground,
    "--thittam-background": colors.background,
    "--thittam-foreground": colors.foreground,
    "--thittam-muted": colors.muted,
    "--thittam-muted-foreground": colors.mutedForeground,
    "--thittam-border": colors.border,

    // Sidebar
    "--thittam-sidebar-bg": colors.sidebar.bg,
    "--thittam-sidebar-text": colors.sidebar.text,
    "--thittam-sidebar-active-item": colors.sidebar.activeItem,
    "--thittam-sidebar-active-bg": colors.sidebar.activeBg,

    // Status indicators
    "--thittam-status-on-track": colors.status.onTrack,
    "--thittam-status-at-risk": colors.status.atRisk,
    "--thittam-status-over-budget": colors.status.overBudget,
  };
}
