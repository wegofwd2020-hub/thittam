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
