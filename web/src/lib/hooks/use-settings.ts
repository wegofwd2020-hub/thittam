"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  listUsers,
  getUser,
  inviteUser,
  updateUserRole,
  deactivateUser,
  listRoles,
  getAuthConfig,
  updateAuthConfig,
  queryAuditLog,
  getTenantSettings,
} from "@/lib/api/settings";

import type {
  InviteUserInput,
  UpdateUserRoleInput,
  UpdateAuthConfigInput,
  AuditLogQuery,
} from "@/lib/api/settings";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const settingsKeys = {
  all: ["settings"] as const,
  users: () => [...settingsKeys.all, "users"] as const,
  userList: () => [...settingsKeys.users(), "list"] as const,
  userDetail: (id: string) => [...settingsKeys.users(), "detail", id] as const,
  roles: () => [...settingsKeys.all, "roles"] as const,
  authConfig: () => [...settingsKeys.all, "auth-config"] as const,
  auditLog: (params?: AuditLogQuery) =>
    [...settingsKeys.all, "audit-log", params] as const,
  tenant: () => [...settingsKeys.all, "tenant"] as const,
};

// ---------------------------------------------------------------------------
// Users — queries
// ---------------------------------------------------------------------------

export function useUsers() {
  return useQuery({
    queryKey: settingsKeys.userList(),
    queryFn: listUsers,
  });
}

export function useUser(id: string) {
  return useQuery({
    queryKey: settingsKeys.userDetail(id),
    queryFn: () => getUser(id),
    enabled: !!id,
  });
}

// ---------------------------------------------------------------------------
// Users — mutations
// ---------------------------------------------------------------------------

export function useInviteUser() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: InviteUserInput) => inviteUser(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.userList() });
      toast.success("Invitation sent");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to send invitation");
    },
  });
}

export function useUpdateUserRole() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateUserRoleInput }) =>
      updateUserRole(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.userList() });
      toast.success("User role updated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to update user role");
    },
  });
}

export function useDeactivateUser() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => deactivateUser(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.userList() });
      toast.success("User deactivated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to deactivate user");
    },
  });
}

// ---------------------------------------------------------------------------
// Roles — queries
// ---------------------------------------------------------------------------

export function useRoles() {
  return useQuery({
    queryKey: settingsKeys.roles(),
    queryFn: listRoles,
    staleTime: 5 * 60 * 1000, // roles rarely change
  });
}

// ---------------------------------------------------------------------------
// Auth Config — queries & mutations
// ---------------------------------------------------------------------------

export function useAuthConfig() {
  return useQuery({
    queryKey: settingsKeys.authConfig(),
    queryFn: getAuthConfig,
    staleTime: 5 * 60 * 1000,
  });
}

export function useUpdateAuthConfig() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: UpdateAuthConfigInput) => updateAuthConfig(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.authConfig() });
      toast.success("Authentication configuration saved");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to update authentication config");
    },
  });
}

// ---------------------------------------------------------------------------
// Audit Log — queries
// ---------------------------------------------------------------------------

export function useAuditLog(params?: AuditLogQuery) {
  return useQuery({
    queryKey: settingsKeys.auditLog(params),
    queryFn: () => queryAuditLog(params),
  });
}

// ---------------------------------------------------------------------------
// Tenant Settings — queries
// ---------------------------------------------------------------------------

export function useTenantSettings() {
  return useQuery({
    queryKey: settingsKeys.tenant(),
    queryFn: getTenantSettings,
    staleTime: 5 * 60 * 1000,
  });
}
