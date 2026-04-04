"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  listProductions,
  getProduction,
  createProduction,
  updateProduction,
  archiveProduction,
  listPhases,
  createPhase,
  updatePhaseStatus,
  getPhaseTypes,
  getEntityLabels,
  listCrewMembers,
  addCrewMember,
  removeCrewMember,
} from "@/lib/api/productions";

import type {
  CreateProductionInput,
  AddCrewMemberInput,
  Production,
} from "@/lib/api/productions";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const productionKeys = {
  all: ["productions"] as const,
  lists: () => [...productionKeys.all, "list"] as const,
  list: (params?: { status?: string }) =>
    [...productionKeys.lists(), params] as const,
  details: () => [...productionKeys.all, "detail"] as const,
  detail: (id: string) => [...productionKeys.details(), id] as const,
  phases: (productionId: string) =>
    [...productionKeys.detail(productionId), "phases"] as const,
  crew: (productionId: string) =>
    [...productionKeys.detail(productionId), "crew"] as const,
  phaseTypes: ["phase-types"] as const,
  entityLabels: ["entity-labels"] as const,
};

// ---------------------------------------------------------------------------
// Productions — queries
// ---------------------------------------------------------------------------

export function useProductions(params?: { status?: string }) {
  return useQuery({
    queryKey: productionKeys.list(params),
    queryFn: () => listProductions(params),
  });
}

export function useProduction(id: string) {
  return useQuery({
    queryKey: productionKeys.detail(id),
    queryFn: () => getProduction(id),
    enabled: !!id,
  });
}

// ---------------------------------------------------------------------------
// Productions — mutations
// ---------------------------------------------------------------------------

export function useCreateProduction() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateProductionInput) => createProduction(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: productionKeys.lists() });
      toast.success("Production created");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to create production");
    },
  });
}

export function useUpdateProduction() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Production> }) =>
      updateProduction(id, data),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: productionKeys.detail(id) });
      qc.invalidateQueries({ queryKey: productionKeys.lists() });
      toast.success("Production updated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to update production");
    },
  });
}

export function useArchiveProduction() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => archiveProduction(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: productionKeys.all });
      toast.success("Production archived");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to archive production");
    },
  });
}

// ---------------------------------------------------------------------------
// Phases — queries
// ---------------------------------------------------------------------------

export function usePhases(productionId: string) {
  return useQuery({
    queryKey: productionKeys.phases(productionId),
    queryFn: () => listPhases(productionId),
    enabled: !!productionId,
  });
}

export function usePhaseTypes() {
  return useQuery({
    queryKey: productionKeys.phaseTypes,
    queryFn: getPhaseTypes,
    staleTime: 5 * 60 * 1000, // phase types rarely change
  });
}

export function useEntityLabels() {
  return useQuery({
    queryKey: productionKeys.entityLabels,
    queryFn: getEntityLabels,
    staleTime: 5 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Phases — mutations
// ---------------------------------------------------------------------------

export function useCreatePhase(productionId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: { phase_type: string }) =>
      createPhase(productionId, data),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: productionKeys.phases(productionId),
      });
      toast.success("Phase created");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to create phase");
    },
  });
}

export function useUpdatePhaseStatus() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      phaseId,
      newPhaseType,
    }: {
      phaseId: string;
      newPhaseType: string;
    }) => updatePhaseStatus(phaseId, newPhaseType),
    onSuccess: () => {
      // Invalidate all production details since we don't know the parent from here
      qc.invalidateQueries({ queryKey: productionKeys.details() });
      toast.success("Phase status updated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to update phase status");
    },
  });
}

// ---------------------------------------------------------------------------
// Crew — queries
// ---------------------------------------------------------------------------

export function useCrewMembers(productionId: string) {
  return useQuery({
    queryKey: productionKeys.crew(productionId),
    queryFn: () => listCrewMembers(productionId),
    enabled: !!productionId,
  });
}

// ---------------------------------------------------------------------------
// Crew — mutations
// ---------------------------------------------------------------------------

export function useAddCrewMember(productionId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: AddCrewMemberInput) =>
      addCrewMember(productionId, data),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: productionKeys.crew(productionId),
      });
      toast.success("Crew member added");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to add crew member");
    },
  });
}

export function useRemoveCrewMember() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => removeCrewMember(id),
    onSuccess: () => {
      // Invalidate all crew queries since we don't have the production ID
      qc.invalidateQueries({ queryKey: productionKeys.all });
      toast.success("Crew member removed");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to remove crew member");
    },
  });
}
