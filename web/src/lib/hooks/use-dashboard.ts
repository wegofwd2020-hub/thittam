"use client";

import { useQuery } from "@tanstack/react-query";

import {
  getPortfolioOverview,
  getFinancialSummary,
  getApprovalPipeline,
  getTeamUtilization,
  getComplianceStatus,
} from "@/lib/api/dashboard";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const dashboardKeys = {
  all: ["dashboard"] as const,
  portfolio: () => [...dashboardKeys.all, "portfolio"] as const,
  financial: () => [...dashboardKeys.all, "financial"] as const,
  approvals: () => [...dashboardKeys.all, "approvals"] as const,
  team: () => [...dashboardKeys.all, "team"] as const,
  compliance: () => [...dashboardKeys.all, "compliance"] as const,
};

// Dashboard data refreshes every 60 seconds
const STALE_TIME = 60 * 1000;

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

export function usePortfolioOverview() {
  return useQuery({
    queryKey: dashboardKeys.portfolio(),
    queryFn: getPortfolioOverview,
    staleTime: STALE_TIME,
  });
}

export function useFinancialSummary() {
  return useQuery({
    queryKey: dashboardKeys.financial(),
    queryFn: getFinancialSummary,
    staleTime: STALE_TIME,
  });
}

export function useApprovalPipeline() {
  return useQuery({
    queryKey: dashboardKeys.approvals(),
    queryFn: getApprovalPipeline,
    staleTime: STALE_TIME,
  });
}

export function useTeamUtilization() {
  return useQuery({
    queryKey: dashboardKeys.team(),
    queryFn: getTeamUtilization,
    staleTime: STALE_TIME,
  });
}

export function useComplianceStatus() {
  return useQuery({
    queryKey: dashboardKeys.compliance(),
    queryFn: getComplianceStatus,
    staleTime: STALE_TIME,
  });
}
