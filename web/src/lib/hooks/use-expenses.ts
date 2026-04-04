"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  listExpenses,
  getExpense,
  submitExpense,
  approveExpense,
  rejectExpense,
  listPurchaseOrders,
  createPurchaseOrder,
  approvePurchaseOrder,
  listPettyCash,
  createPettyCash,
  settlePettyCash,
  getExpenseCategories,
  getApprovalLimits,
} from "@/lib/api/expenses";

import type {
  SubmitExpenseInput,
  CreatePurchaseOrderInput,
  CreatePettyCashInput,
  SettlePettyCashInput,
} from "@/lib/api/expenses";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const expenseKeys = {
  all: ["expenses"] as const,
  lists: () => [...expenseKeys.all, "list"] as const,
  list: (productionId: string, params?: { status?: string }) =>
    [...expenseKeys.lists(), productionId, params] as const,
  details: () => [...expenseKeys.all, "detail"] as const,
  detail: (id: string) => [...expenseKeys.details(), id] as const,
  purchaseOrders: ["purchase-orders"] as const,
  poLists: () => [...expenseKeys.purchaseOrders, "list"] as const,
  poList: (productionId: string, params?: { status?: string }) =>
    [...expenseKeys.poLists(), productionId, params] as const,
  pettyCash: ["petty-cash"] as const,
  pettyCashLists: () => [...expenseKeys.pettyCash, "list"] as const,
  pettyCashList: (productionId: string, params?: { status?: string }) =>
    [...expenseKeys.pettyCashLists(), productionId, params] as const,
  categories: ["expense-categories"] as const,
  approvalLimits: ["approval-limits"] as const,
};

// ---------------------------------------------------------------------------
// Expenses — queries
// ---------------------------------------------------------------------------

export function useExpenses(
  productionId: string,
  params?: { status?: string },
) {
  return useQuery({
    queryKey: expenseKeys.list(productionId, params),
    queryFn: () => listExpenses(productionId, params),
    enabled: !!productionId,
  });
}

export function useExpense(id: string) {
  return useQuery({
    queryKey: expenseKeys.detail(id),
    queryFn: () => getExpense(id),
    enabled: !!id,
  });
}

// ---------------------------------------------------------------------------
// Expenses — mutations
// ---------------------------------------------------------------------------

export function useSubmitExpense() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: SubmitExpenseInput) => submitExpense(data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: expenseKeys.list(variables.production_id),
      });
      toast.success("Expense submitted");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to submit expense");
    },
  });
}

export function useApproveExpense() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => approveExpense(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: expenseKeys.detail(data.id) });
      qc.invalidateQueries({ queryKey: expenseKeys.lists() });
      toast.success("Expense approved");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to approve expense");
    },
  });
}

export function useRejectExpense() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      rejectExpense(id, reason),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: expenseKeys.detail(data.id) });
      qc.invalidateQueries({ queryKey: expenseKeys.lists() });
      toast.success("Expense rejected");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to reject expense");
    },
  });
}

// ---------------------------------------------------------------------------
// Purchase Orders — queries
// ---------------------------------------------------------------------------

export function usePurchaseOrders(
  productionId: string,
  params?: { status?: string },
) {
  return useQuery({
    queryKey: expenseKeys.poList(productionId, params),
    queryFn: () => listPurchaseOrders(productionId, params),
    enabled: !!productionId,
  });
}

// ---------------------------------------------------------------------------
// Purchase Orders — mutations
// ---------------------------------------------------------------------------

export function useCreatePurchaseOrder() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreatePurchaseOrderInput) => createPurchaseOrder(data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: expenseKeys.poList(variables.production_id),
      });
      toast.success("Purchase order created");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to create purchase order");
    },
  });
}

export function useApprovePurchaseOrder() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => approvePurchaseOrder(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: expenseKeys.poLists() });
      toast.success("Purchase order approved");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to approve purchase order");
    },
  });
}

// ---------------------------------------------------------------------------
// Petty Cash — queries
// ---------------------------------------------------------------------------

export function usePettyCash(
  productionId: string,
  params?: { status?: string },
) {
  return useQuery({
    queryKey: expenseKeys.pettyCashList(productionId, params),
    queryFn: () => listPettyCash(productionId, params),
    enabled: !!productionId,
  });
}

// ---------------------------------------------------------------------------
// Petty Cash — mutations
// ---------------------------------------------------------------------------

export function useCreatePettyCash() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreatePettyCashInput) => createPettyCash(data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: expenseKeys.pettyCashList(variables.production_id),
      });
      toast.success("Petty cash advance issued");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to issue petty cash advance");
    },
  });
}

export function useSettlePettyCash(productionId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: SettlePettyCashInput }) =>
      settlePettyCash(id, data),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: expenseKeys.pettyCashList(productionId),
      });
      toast.success("Petty cash settled");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to settle petty cash");
    },
  });
}

// ---------------------------------------------------------------------------
// Categories & Approval config — queries
// ---------------------------------------------------------------------------

export function useExpenseCategories() {
  return useQuery({
    queryKey: expenseKeys.categories,
    queryFn: getExpenseCategories,
    staleTime: 5 * 60 * 1000, // categories rarely change
  });
}

export function useApprovalLimits() {
  return useQuery({
    queryKey: expenseKeys.approvalLimits,
    queryFn: getApprovalLimits,
    staleTime: 5 * 60 * 1000, // approval config rarely changes
  });
}
