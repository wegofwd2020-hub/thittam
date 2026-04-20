"use client";

import { useMemo } from "react";
import { useBudgets, useLineItems } from "./use-budgets";
import type { Budget, BudgetLineItem } from "@/lib/api/budgets";

export type BudgetHealth = "on_track" | "at_risk" | "over_budget";

export interface ProductionBudgetSummary {
  budget: Budget;
  totalBudgeted: number;
  totalActual: number;
  totalCommitted: number;
  currency: string;
  utilizationPct: number; // can exceed 100 when over budget
  remaining: number; // budgeted - actual - committed
  health: BudgetHealth;
}

// Preference order for picking a production's "effective" budget version:
// locked (finalised) > approved > submitted > draft. Within the same
// status, the most recent updated_at wins so a tenant that has re-
// approved a new version sees the new numbers.
const STATUS_PRIORITY: Record<string, number> = {
  locked: 4,
  approved: 3,
  submitted: 2,
  draft: 1,
};

function selectEffectiveBudget(budgets: Budget[] | undefined): Budget | null {
  if (!budgets || budgets.length === 0) return null;
  return [...budgets].sort((a, b) => {
    const pa = STATUS_PRIORITY[a.status] ?? 0;
    const pb = STATUS_PRIORITY[b.status] ?? 0;
    if (pa !== pb) return pb - pa;
    return b.updated_at.localeCompare(a.updated_at);
  })[0];
}

// API money comes in as string decimals (Rule #1). For client-side display-
// only aggregation we parseFloat and sum with JS numbers — lossless up to
// ~Number.MAX_SAFE_INTEGER / 100 (≈ $90 quadrillion), well above any
// realistic production budget. If we ever need cent-precision discipline
// client-side, swap in decimal.js.
function sumField(
  items: BudgetLineItem[],
  pick: (li: BudgetLineItem) => string,
): number {
  return items.reduce(
    (acc, li) => acc + (Number.parseFloat(pick(li)) || 0),
    0,
  );
}

function computeHealth(utilizationPct: number): BudgetHealth {
  if (utilizationPct > 100) return "over_budget";
  if (utilizationPct >= 80) return "at_risk";
  return "on_track";
}

/**
 * Aggregates the "effective" budget for a production into a single summary:
 * pick the highest-priority budget version, fetch its line items, sum the
 * budgeted / actual / committed columns, derive utilization and health.
 *
 * Returns `data: null` when the production has no budget versions yet —
 * callers should handle that separately from the loading state.
 */
export function useProductionBudgetSummary(productionId: string) {
  const budgetsQuery = useBudgets(productionId);

  const effective = useMemo(
    () => selectEffectiveBudget(budgetsQuery.data?.budgets),
    [budgetsQuery.data],
  );

  // useLineItems gates itself on a truthy budgetId, so passing "" when
  // there is no effective budget keeps the query disabled.
  const lineItemsQuery = useLineItems(effective?.id ?? "");

  const summary = useMemo<ProductionBudgetSummary | null>(() => {
    if (!effective || !lineItemsQuery.data) return null;
    const items = lineItemsQuery.data;
    const totalBudgeted = sumField(items, (li) => li.budgeted_amount);
    const totalActual = sumField(items, (li) => li.actual_amount);
    const totalCommitted = sumField(items, (li) => li.committed_amount);
    const remaining = totalBudgeted - totalActual - totalCommitted;
    const utilizationPct =
      totalBudgeted > 0 ? (totalActual / totalBudgeted) * 100 : 0;
    return {
      budget: effective,
      totalBudgeted,
      totalActual,
      totalCommitted,
      currency: effective.currency,
      utilizationPct,
      remaining,
      health: computeHealth(utilizationPct),
    };
  }, [effective, lineItemsQuery.data]);

  return {
    data: summary,
    hasBudget: !!effective,
    isLoading:
      budgetsQuery.isLoading || (!!effective && lineItemsQuery.isLoading),
    error: budgetsQuery.error ?? lineItemsQuery.error,
  };
}
