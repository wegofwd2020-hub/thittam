"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  generateDemoPreview,
  provisionDemo,
  listDemoTenants,
  teardownDemoTenant,
  listTenants,
  suspendTenant,
  reactivateTenant,
  upgradePlan,
  listVerticals,
  deprecateVertical,
} from "@/lib/api/platform";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const platformKeys = {
  all: ["platform"] as const,
  demoTenants: () => [...platformKeys.all, "demo-tenants"] as const,
  tenants: () => [...platformKeys.all, "tenants"] as const,
  verticals: () => [...platformKeys.all, "verticals"] as const,
};

// ---------------------------------------------------------------------------
// Demo provisioning — mutations
// ---------------------------------------------------------------------------

export function useGenerateDemoPreview() {
  return useMutation({
    mutationFn: ({
      verticalId,
      companyName,
      plan,
    }: {
      verticalId: string;
      companyName?: string;
      plan?: string;
    }) => generateDemoPreview(verticalId, companyName, plan),
    onError: (err: Error) => {
      toast.error(err.message || "Failed to generate demo preview");
    },
  });
}

export function useProvisionDemo() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      previewToken,
      verticalId,
      companyName,
      plan,
    }: {
      previewToken: string;
      verticalId: string;
      companyName: string;
      plan: string;
    }) => provisionDemo(previewToken, verticalId, companyName, plan),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: platformKeys.demoTenants() });
      toast.success("Demo tenant provisioned");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to provision demo tenant");
    },
  });
}

// ---------------------------------------------------------------------------
// Demo tenants — queries & mutations
// ---------------------------------------------------------------------------

export function useDemoTenants() {
  return useQuery({
    queryKey: platformKeys.demoTenants(),
    queryFn: listDemoTenants,
  });
}

export function useTeardownDemoTenant() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (tenantId: string) => teardownDemoTenant(tenantId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: platformKeys.demoTenants() });
      toast.success("Demo tenant removed");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to remove demo tenant");
    },
  });
}

// ---------------------------------------------------------------------------
// Tenants — queries & mutations
// ---------------------------------------------------------------------------

export function useTenants() {
  return useQuery({
    queryKey: platformKeys.tenants(),
    queryFn: listTenants,
  });
}

export function useSuspendTenant() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => suspendTenant(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: platformKeys.tenants() });
      toast.success("Tenant suspended");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to suspend tenant");
    },
  });
}

export function useReactivateTenant() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => reactivateTenant(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: platformKeys.tenants() });
      toast.success("Tenant reactivated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to reactivate tenant");
    },
  });
}

export function useUpgradePlan() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, plan }: { id: string; plan: string }) =>
      upgradePlan(id, plan),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: platformKeys.tenants() });
      toast.success("Plan upgraded");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to upgrade plan");
    },
  });
}

// ---------------------------------------------------------------------------
// Verticals — queries & mutations
// ---------------------------------------------------------------------------

export function useVerticals() {
  return useQuery({
    queryKey: platformKeys.verticals(),
    queryFn: listVerticals,
    staleTime: 5 * 60 * 1000,
  });
}

export function useDeprecateVertical() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => deprecateVertical(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: platformKeys.verticals() });
      toast.success("Vertical deprecated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to deprecate vertical");
    },
  });
}
