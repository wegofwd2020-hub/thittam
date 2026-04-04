"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  getThemeOverride,
  saveThemeOverride,
  resetThemeOverride,
  uploadLogo,
  uploadFavicon,
} from "@/lib/api/theme";

import type { ThemeOverride } from "@/lib/api/theme";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const themeKeys = {
  all: ["theme"] as const,
  override: () => [...themeKeys.all, "override"] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export function useThemeOverride() {
  return useQuery({
    queryKey: themeKeys.override(),
    queryFn: getThemeOverride,
    staleTime: 5 * 60 * 1000,
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export function useSaveThemeOverride() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (override: ThemeOverride) => saveThemeOverride(override),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: themeKeys.override() });
      toast.success("Theme saved successfully");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to save theme");
    },
  });
}

export function useResetThemeOverride() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => resetThemeOverride(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: themeKeys.override() });
      toast.success("Theme reset to vertical default");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to reset theme");
    },
  });
}

export function useUploadLogo() {
  return useMutation({
    mutationFn: (file: File) => uploadLogo(file),
    onSuccess: () => {
      toast.success("Logo uploaded");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to upload logo");
    },
  });
}

export function useUploadFavicon() {
  return useMutation({
    mutationFn: (file: File) => uploadFavicon(file),
    onSuccess: () => {
      toast.success("Favicon uploaded");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to upload favicon");
    },
  });
}
