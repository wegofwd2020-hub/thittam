"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  listBudgets,
  getBudget,
  createBudget,
  createBudgetFromTemplate,
  submitBudget,
  approveBudget,
  listLineItems,
  createLineItem,
  updateLineItem,
  deleteLineItem,
  checkLineAvailability,
  getBudgetCategories,
  getBudgetTemplates,
} from "@/lib/api/budgets";

import type {
  CreateBudgetInput,
  CreateBudgetFromTemplateInput,
  CreateLineItemInput,
  UpdateLineItemInput,
} from "@/lib/api/budgets";

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const budgetKeys = {
  all: ["budgets"] as const,
  lists: () => [...budgetKeys.all, "list"] as const,
  list: (productionId: string, params?: { status?: string }) =>
    [...budgetKeys.lists(), productionId, params] as const,
  details: () => [...budgetKeys.all, "detail"] as const,
  detail: (id: string) => [...budgetKeys.details(), id] as const,
  lineItems: (budgetId: string) =>
    [...budgetKeys.detail(budgetId), "line-items"] as const,
  lineAvailability: (lineId: string) =>
    [...budgetKeys.all, "availability", lineId] as const,
  categories: ["budget-categories"] as const,
  templates: ["budget-templates"] as const,
};

// ---------------------------------------------------------------------------
// Budgets — queries
// ---------------------------------------------------------------------------

export function useBudgets(
  productionId: string,
  params?: { status?: string },
) {
  return useQuery({
    queryKey: budgetKeys.list(productionId, params),
    queryFn: () => listBudgets(productionId, params),
    enabled: !!productionId,
  });
}

export function useBudget(id: string) {
  return useQuery({
    queryKey: budgetKeys.detail(id),
    queryFn: () => getBudget(id),
    enabled: !!id,
  });
}

// ---------------------------------------------------------------------------
// Budgets — mutations
// ---------------------------------------------------------------------------

export function useCreateBudget() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateBudgetInput) => createBudget(data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: budgetKeys.list(variables.production_id),
      });
      toast.success("Budget created");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to create budget");
    },
  });
}

export function useCreateBudgetFromTemplate() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateBudgetFromTemplateInput) =>
      createBudgetFromTemplate(data),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({
        queryKey: budgetKeys.list(variables.production_id),
      });
      toast.success("Budget created from template");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to create budget from template");
    },
  });
}

export function useSubmitBudget() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => submitBudget(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: budgetKeys.detail(data.id) });
      qc.invalidateQueries({ queryKey: budgetKeys.lists() });
      toast.success("Budget submitted for approval");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to submit budget");
    },
  });
}

export function useApproveBudget() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => approveBudget(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: budgetKeys.detail(data.id) });
      qc.invalidateQueries({ queryKey: budgetKeys.lists() });
      toast.success("Budget approved");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to approve budget");
    },
  });
}

// ---------------------------------------------------------------------------
// Line Items — queries
// ---------------------------------------------------------------------------

export function useLineItems(budgetId: string) {
  return useQuery({
    queryKey: budgetKeys.lineItems(budgetId),
    queryFn: () => listLineItems(budgetId),
    enabled: !!budgetId,
  });
}

export function useLineAvailability(lineId: string) {
  return useQuery({
    queryKey: budgetKeys.lineAvailability(lineId),
    queryFn: () => checkLineAvailability(lineId),
    enabled: !!lineId,
  });
}

// ---------------------------------------------------------------------------
// Line Items — mutations
// ---------------------------------------------------------------------------

export function useCreateLineItem(budgetId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateLineItemInput) =>
      createLineItem(budgetId, data),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: budgetKeys.lineItems(budgetId),
      });
      qc.invalidateQueries({
        queryKey: budgetKeys.detail(budgetId),
      });
      toast.success("Line item added");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to add line item");
    },
  });
}

export function useUpdateLineItem(budgetId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateLineItemInput }) =>
      updateLineItem(id, data),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: budgetKeys.lineItems(budgetId),
      });
      qc.invalidateQueries({
        queryKey: budgetKeys.detail(budgetId),
      });
      toast.success("Line item updated");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to update line item");
    },
  });
}

export function useDeleteLineItem(budgetId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => deleteLineItem(id),
    onSuccess: () => {
      qc.invalidateQueries({
        queryKey: budgetKeys.lineItems(budgetId),
      });
      qc.invalidateQueries({
        queryKey: budgetKeys.detail(budgetId),
      });
      toast.success("Line item removed");
    },
    onError: (err: Error) => {
      toast.error(err.message || "Failed to remove line item");
    },
  });
}

// ---------------------------------------------------------------------------
// Categories & Templates — queries
// ---------------------------------------------------------------------------

export function useBudgetCategories() {
  return useQuery({
    queryKey: budgetKeys.categories,
    queryFn: getBudgetCategories,
    staleTime: 5 * 60 * 1000, // categories rarely change
  });
}

export function useBudgetTemplates() {
  return useQuery({
    queryKey: budgetKeys.templates,
    queryFn: getBudgetTemplates,
    staleTime: 5 * 60 * 1000, // templates rarely change
  });
}
