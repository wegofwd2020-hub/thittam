export interface SidebarColors {
  bg: string;
  text: string;
  activeItem: string;
  activeBg: string;
}

export interface StatusColors {
  onTrack: string;
  atRisk: string;
  overBudget: string;
}

export interface ThemeColors {
  primary: string;
  primaryForeground: string;
  secondary: string;
  secondaryForeground: string;
  accent: string;
  accentForeground: string;
  background: string;
  foreground: string;
  muted: string;
  mutedForeground: string;
  border: string;
  sidebar: SidebarColors;
  status: StatusColors;
}

export interface ThemeIcons {
  sidebar: Record<string, string>;
  // Per-vertical entity icons (grouped by project type)
  entities: Record<string, string>;
}

// Typography configuration
export interface ThemeTypography {
  headingFont: string; // CSS font-family for headings/labels (default: Inter)
  bodyFont: string;    // CSS font-family for body text (default: Merriweather)
  monoFont: string;    // CSS font-family for numbers/codes (default: JetBrains Mono)
}

export interface ThemeBranding {
  logo?: string;
  favicon?: string;
  loginBg?: string;
}

export interface ThittamTheme {
  id: string;
  name: string;
  colors: ThemeColors;
  icons: ThemeIcons;
  branding: ThemeBranding;
  typography: ThemeTypography;
}

export interface EntityLabels {
  project: string;
  projectPlural: string;
  phase: string;
  phasePlural: string;
  teamMember: string;
  teamMemberPlural: string;
  rateLabel: string;
  rateUnit: string;
}

export interface VerticalDefinition {
  theme: ThittamTheme;
  entityLabels: EntityLabels;
}
