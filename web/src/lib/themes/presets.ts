import type { ThemeColors } from "./types";

/**
 * Theme presets shipped with #65. These are partial overrides applied on top
 * of the current vertical's default theme — the vertical still controls icons,
 * entity labels, and sidebar chrome; the preset only re-skins the core palette.
 *
 * A preset pick is persisted by the existing tenant theme-override API
 * (see useSaveThemeOverride). Tenant-branded themes are a follow-up.
 */

export interface ThemePreset {
  id: string;
  name: string;
  description: string;
  /** Partial override merged onto the vertical's default colors. */
  colors: Partial<ThemeColors>;
}

export const THEME_PRESETS: ThemePreset[] = [
  {
    id: "default",
    name: "Default",
    description: "Vertical-driven colors — no override.",
    colors: {},
  },
  {
    id: "slate",
    name: "Slate",
    description: "Neutral slate primary, low chroma.",
    colors: {
      primary: "#334155",
      primaryForeground: "#F8FAFC",
      accent: "#64748B",
      accentForeground: "#F8FAFC",
    },
  },
  {
    id: "rose",
    name: "Rose",
    description: "Warm rose primary, soft accents.",
    colors: {
      primary: "#E11D48",
      primaryForeground: "#FFF1F2",
      accent: "#F43F5E",
      accentForeground: "#FFF1F2",
    },
  },
  {
    id: "kaundinya-labs",
    name: "Kaundinya Labs",
    description: "Navy ink with a saffron accent — the Kaundinya Labs house look.",
    colors: {
      primary: "#16213A",
      primaryForeground: "#FFFFFF",
      secondary: "#243150",
      secondaryForeground: "#D5DAE4",
      accent: "#A3541A",
      accentForeground: "#FFFFFF",
      background: "#FFFFFF",
      foreground: "#16213A",
      muted: "#EEF0F4",
      mutedForeground: "#4A5368",
      border: "#C9CDD6",
      sidebar: {
        bg: "#16213A",
        text: "#D5DAE4",
        activeItem: "#E08A3C",
        activeBg: "#243150",
      },
      status: {
        onTrack: "#15803D",
        atRisk: "#B45309",
        overBudget: "#B91C1C",
      },
    },
  },
];
