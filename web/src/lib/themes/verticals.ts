import type { VerticalDefinition } from "./types";

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
      sidebar: {
        bg: "#1E1B2E",
        text: "#CBD5E1",
        activeItem: "#F59E0B",
        activeBg: "#2D2A3E",
      },
      status: {
        onTrack: "#16A34A",
        atRisk: "#F59E0B",
        overBudget: "#DC2626",
      },
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
    },
    branding: {},
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
      sidebar: {
        bg: "#0F172A",
        text: "#94A3B8",
        activeItem: "#38BDF8",
        activeBg: "#1E293B",
      },
      status: {
        onTrack: "#16A34A",
        atRisk: "#EAB308",
        overBudget: "#DC2626",
      },
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
    },
    branding: {},
  },
  entityLabels: {
    project: "Project",
    projectPlural: "Projects",
    phase: "Sprint",
    phasePlural: "Sprints",
    teamMember: "Developer",
    teamMemberPlural: "Developers",
    rateLabel: "Hourly Rate",
    rateUnit: "/hr",
  },
};

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
      sidebar: {
        bg: "#1C1917",
        text: "#A8A29E",
        activeItem: "#F59E0B",
        activeBg: "#292524",
      },
      status: {
        onTrack: "#16A34A",
        atRisk: "#F59E0B",
        overBudget: "#DC2626",
      },
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
    },
    branding: {},
  },
  entityLabels: {
    project: "Job Site",
    projectPlural: "Job Sites",
    phase: "Phase",
    phasePlural: "Phases",
    teamMember: "Worker",
    teamMemberPlural: "Workers",
    rateLabel: "Day Rate",
    rateUnit: "/day",
  },
};

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
      sidebar: {
        bg: "#1E1033",
        text: "#C4B5FD",
        activeItem: "#EC4899",
        activeBg: "#2D1F4E",
      },
      status: {
        onTrack: "#16A34A",
        atRisk: "#F59E0B",
        overBudget: "#DC2626",
      },
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
    },
    branding: {},
  },
  entityLabels: {
    project: "Event",
    projectPlural: "Events",
    phase: "Stage",
    phasePlural: "Stages",
    teamMember: "Staff Member",
    teamMemberPlural: "Staff Members",
    rateLabel: "Hourly Rate",
    rateUnit: "/hr",
  },
};

export const verticals: Record<string, VerticalDefinition> = {
  "movie-production": movieProduction,
  "software-development": softwareDevelopment,
  "construction": construction,
  "events-management": eventsManagement,
};

export function getVerticalDefinition(
  verticalId: string,
): VerticalDefinition | undefined {
  return verticals[verticalId];
}

export function getVerticalTheme(verticalId: string) {
  return verticals[verticalId]?.theme;
}

export function getEntityLabels(verticalId: string) {
  return verticals[verticalId]?.entityLabels;
}
