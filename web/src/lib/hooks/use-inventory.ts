"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  listAssets,
  getAsset,
  createAsset,
  listCheckouts,
  checkOutAsset,
  checkInAsset,
  getInventoryCategories,
} from "@/lib/api/inventory";

import type {
  CreateAssetInput,
  CheckOutAssetInput,
  CheckInAssetInput,
} from "@/lib/api/inventory";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const inventoryKeys = {
  all: ["inventory"] as const,
  lists: () => [...inventoryKeys.all, "list"] as const,
  list: (params?: { status?: string; category_id?: string; search?: string }) =>
    [...inventoryKeys.lists(), params] as const,
  details: () => [...inventoryKeys.all, "detail"] as const,
  detail: (id: string) => [...inventoryKeys.details(), id] as const,
  checkouts: (assetId: string) =>
    [...inventoryKeys.all, "checkouts", assetId] as const,
  categories: ["inventory-categories"] as const,
};

// ---------------------------------------------------------------------------
// Assets — queries
// ---------------------------------------------------------------------------

export function useAssets(params?: {
  status?: string;
  category_id?: string;
  search?: string;
}) {
  return useQuery({
    queryKey: inventoryKeys.list(params),
    queryFn: () => listAssets(params),
  });
}

export function useAsset(id: string) {
  return useQuery({
    queryKey: inventoryKeys.detail(id),
    queryFn: () => getAsset(id),
    enabled: !!id,
  });
}

// ---------------------------------------------------------------------------
// Assets — mutations
// ---------------------------------------------------------------------------

export function useCreateAsset() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateAssetInput) => createAsset(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: inventoryKeys.lists() });
      toast.success("Asset created");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to create asset");
    },
  });
}

// ---------------------------------------------------------------------------
// Checkouts — queries
// ---------------------------------------------------------------------------

export function useCheckouts(assetId: string) {
  return useQuery({
    queryKey: inventoryKeys.checkouts(assetId),
    queryFn: () => listCheckouts(assetId),
    enabled: !!assetId,
  });
}

// ---------------------------------------------------------------------------
// Checkouts — mutations
// ---------------------------------------------------------------------------

export function useCheckOutAsset() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      assetId,
      data,
    }: {
      assetId: string;
      data: CheckOutAssetInput;
    }) => checkOutAsset(assetId, data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: inventoryKeys.detail(variables.assetId),
      });
      qc.invalidateQueries({
        queryKey: inventoryKeys.checkouts(variables.assetId),
      });
      qc.invalidateQueries({ queryKey: inventoryKeys.lists() });
      toast.success("Asset checked out");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to check out asset");
    },
  });
}

export function useCheckInAsset() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      assetId,
      data,
    }: {
      assetId: string;
      data: CheckInAssetInput;
    }) => checkInAsset(assetId, data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: inventoryKeys.detail(variables.assetId),
      });
      qc.invalidateQueries({
        queryKey: inventoryKeys.checkouts(variables.assetId),
      });
      qc.invalidateQueries({ queryKey: inventoryKeys.lists() });
      toast.success("Asset checked in");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to check in asset");
    },
  });
}

// ---------------------------------------------------------------------------
// Categories — queries
// ---------------------------------------------------------------------------

export function useInventoryCategories() {
  return useQuery({
    queryKey: inventoryKeys.categories,
    queryFn: getInventoryCategories,
    staleTime: 5 * 60 * 1000, // categories rarely change
  });
}
