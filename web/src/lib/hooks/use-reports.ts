"use client";

import { useQuery } from "@tanstack/react-query";
import {
  listReportDefinitions,
  getReportDefinition,
  runReport,
} from "@/lib/api/reports";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const reportKeys = {
  all: ["reports"] as const,
  definitions: (verticalId?: string) =>
    [...reportKeys.all, "definitions", verticalId] as const,
  definition: (id: string) => [...reportKeys.all, "definition", id] as const,
  data: (id: string, params?: Record<string, string | undefined>) =>
    [...reportKeys.all, "data", id, params] as const,
};

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

export function useReportDefinitions(verticalId?: string) {
  return useQuery({
    queryKey: reportKeys.definitions(verticalId),
    queryFn: () => listReportDefinitions(verticalId),
    staleTime: 5 * 60 * 1000,
  });
}

export function useReportDefinition(id: string) {
  return useQuery({
    queryKey: reportKeys.definition(id),
    queryFn: () => getReportDefinition(id),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
  });
}

export function useReportData(
  id: string,
  params: { production_id?: string; from?: string; to?: string },
) {
  return useQuery({
    queryKey: reportKeys.data(id, params),
    queryFn: () => runReport(id, params),
    enabled: !!id,
  });
}
