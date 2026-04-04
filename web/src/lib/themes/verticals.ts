import type { VerticalDefinition, ThemeTypography } from "./types";

// Default typography — all verticals share this unless overridden.
const defaultTypography: ThemeTypography = {
  headingFont: "var(--font-heading), 'Inter', system-ui, sans-serif",
  bodyFont: "var(--font-body), 'Merriweather', Georgia, serif",
  monoFont: "var(--font-mono), 'JetBrains Mono', Consolas, monospace",
};

// ── Movie Production ──────────────────────────────────────────────────
const movieProduction: VerticalDefinition = {
  theme: {
    id: "movie-production",
    name: "Movie Production",
    colors: {
      primary: "#DC2626",
      primaryForeground: "#FFFFFF",
      secondary: "#1E293B",
      secondaryForeground: "#F8FAFC",
      accent: "#F59E0B",
      accentForeground: "#1C1917",
      background: "#FFFFFF",
      foreground: "#0F172A",
      muted: "#F1F5F9",
      mutedForeground: "#64748B",
      border: "#E2E8F0",
      sidebar: { bg: "#1E1B2E", text: "#CBD5E1", activeItem: "#F59E0B", activeBg: "#2D2A3E" },
      status: { onTrack: "#16A34A", atRisk: "#F59E0B", overBudget: "#DC2626" },
    },
    icons: {
      sidebar: {
        dashboard: "LayoutDashboard",
        productions: "Clapperboard",
        budgets: "DollarSign",
        expenses: "Receipt",
        inventory: "Warehouse",
        reports: "BarChart3",
        team: "Users",
        settings: "Settings",
      },
      entities: {
        project: "Clapperboard",
        phase: "Film",
        teamMember: "Camera",
        budget: "DollarSign",
        expense: "Receipt",
        asset: "Video",
        report: "BarChart3",
        director: "Megaphone",
        talent: "Star",
        ticket: "Ticket",
      },
    },
    branding: {},
    typography: defaultTypography,
  },
  entityLabels: {
    project: "Production",
    projectPlural: "Productions",
    phase: "Phase",
    phasePlural: "Phases",
    teamMember: "Crew Member",
    teamMemberPlural: "Crew Members",
    rateLabel: "Day Rate",
    rateUnit: "/day",
  },
};

// ── Software Development ──────────────────────────────────────────────
const softwareDevelopment: VerticalDefinition = {
  theme: {
    id: "software-development",
    name: "Software Development",
    colors: {
      primary: "#2563EB",
      primaryForeground: "#FFFFFF",
      secondary: "#1E293B",
      secondaryForeground: "#F8FAFC",
      accent: "#06B6D4",
      accentForeground: "#FFFFFF",
      background: "#FFFFFF",
      foreground: "#0F172A",
      muted: "#F1F5F9",
      mutedForeground: "#64748B",
      border: "#E2E8F0",
      sidebar: { bg: "#0F172A", text: "#94A3B8", activeItem: "#38BDF8", activeBg: "#1E293B" },
      status: { onTrack: "#16A34A", atRisk: "#EAB308", overBudget: "#DC2626" },
    },
    icons: {
      sidebar: {
        dashboard: "LayoutDashboard",
        productions: "FolderKanban",
        budgets: "DollarSign",
        expenses: "Receipt",
        inventory: "Server",
        reports: "BarChart3",
        team: "Users",
        settings: "Settings",
      },
      entities: {
        project: "FolderKanban",
        phase: "GitBranch",
        teamMember: "Code",
        budget: "DollarSign",
        expense: "Receipt",
        asset: "Monitor",
        report: "BarChart3",
        sprint: "Terminal",
        bug: "Bug",
        deploy: "Rocket",
        cloud: "Cloud",
      },
    },
    branding: {},
    typography: defaultTypography,
  },
  entityLabels: {
    project: "Project",
    projectPlural: "Projects",
    phase: "Milestone",
    phasePlural: "Milestones",
    teamMember: "Team Member",
    teamMemberPlural: "Team Members",
    rateLabel: "Hourly Rate",
    rateUnit: "/hr",
  },
};

// ── Construction ──────────────────────────────────────────────────────
const construction: VerticalDefinition = {
  theme: {
    id: "construction",
    name: "Construction",
    colors: {
      primary: "#D97706",
      primaryForeground: "#FFFFFF",
      secondary: "#475569",
      secondaryForeground: "#F8FAFC",
      accent: "#475569",
      accentForeground: "#FFFFFF",
      background: "#FFFFFF",
      foreground: "#0F172A",
      muted: "#F1F5F9",
      mutedForeground: "#64748B",
      border: "#E2E8F0",
      sidebar: { bg: "#1C1917", text: "#A8A29E", activeItem: "#F59E0B", activeBg: "#292524" },
      status: { onTrack: "#16A34A", atRisk: "#F59E0B", overBudget: "#DC2626" },
    },
    icons: {
      sidebar: {
        dashboard: "LayoutDashboard",
        productions: "Building2",
        budgets: "DollarSign",
        expenses: "Receipt",
        inventory: "HardHat",
        reports: "BarChart3",
        team: "Users",
        settings: "Settings",
      },
      entities: {
        project: "Building2",
        phase: "Hammer",
        teamMember: "HardHat",
        budget: "DollarSign",
        expense: "Receipt",
        asset: "Truck",
        report: "BarChart3",
        blueprint: "Ruler",
        site: "Landmark",
        material: "Warehouse",
        crane: "Wrench",
      },
    },
    branding: {},
    typography: defaultTypography,
  },
  entityLabels: {
    project: "Contract",
    projectPlural: "Contracts",
    phase: "Work Package",
    phasePlural: "Work Packages",
    teamMember: "Site Worker",
    teamMemberPlural: "Site Workers",
    rateLabel: "Day Rate",
    rateUnit: "/day",
  },
};

// ── Events Management ─────────────────────────────────────────────────
const eventsManagement: VerticalDefinition = {
  theme: {
    id: "events-management",
    name: "Events Management",
    colors: {
      primary: "#7C3AED",
      primaryForeground: "#FFFFFF",
      secondary: "#1E293B",
      secondaryForeground: "#F8FAFC",
      accent: "#EC4899",
      accentForeground: "#FFFFFF",
      background: "#FFFFFF",
      foreground: "#0F172A",
      muted: "#F1F5F9",
      mutedForeground: "#64748B",
      border: "#E2E8F0",
      sidebar: { bg: "#1E1033", text: "#C4B5FD", activeItem: "#EC4899", activeBg: "#2D1F4E" },
      status: { onTrack: "#16A34A", atRisk: "#F59E0B", overBudget: "#DC2626" },
    },
    icons: {
      sidebar: {
        dashboard: "LayoutDashboard",
        productions: "CalendarDays",
        budgets: "DollarSign",
        expenses: "Receipt",
        inventory: "Package",
        reports: "BarChart3",
        team: "Users",
        settings: "Settings",
      },
      entities: {
        project: "CalendarDays",
        phase: "PartyPopper",
        teamMember: "Users",
        budget: "DollarSign",
        expense: "Receipt",
        asset: "Package",
        report: "BarChart3",
        venue: "MapPin",
        performer: "Mic",
        music: "Music",
        gift: "Gift",
        sparkle: "Sparkles",
      },
    },
    branding: {},
    typography: defaultTypography,
  },
  entityLabels: {
    project: "Event",
    projectPlural: "Events",
    phase: "Stage",
    phasePlural: "Stages",
    teamMember: "Event Staff",
    teamMemberPlural: "Event Staff",
    rateLabel: "Day Rate",
    rateUnit: "/day",
  },
};

export const verticals: Record<string, VerticalDefinition> = {
  "movie-production": movieProduction,
  "software-development": softwareDevelopment,
  "construction": construction,
  "events-management": eventsManagement,
};

export function getVerticalDefinition(verticalId: string): VerticalDefinition | undefined {
  return verticals[verticalId];
}

export function getVerticalTheme(verticalId: string) {
  return verticals[verticalId]?.theme;
}

export function getEntityLabels(verticalId: string) {
  return verticals[verticalId]?.entityLabels;
}
