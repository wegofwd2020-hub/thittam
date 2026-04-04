"use client";

import { useMemo } from "react";
import { Pencil, Trash2, Plus } from "lucide-react";
import { VarianceIndicator } from "./variance-indicator";
import type { BudgetLineItem, BudgetCategory } from "@/lib/api/budgets";

// ---------------------------------------------------------------------------
// LineItemTable — editable budget line items table with totals footer.
// ---------------------------------------------------------------------------

interface LineItemTableProps {
  items: BudgetLineItem[];
  categories: BudgetCategory[];
  isEditable: boolean; // false when budget is approved/locked
  onAdd?: () => void;
  onEdit?: (item: BudgetLineItem) => void;
  onDelete?: (id: string) => void;
}

function parseDecimal(value: string): number {
  const num = parseFloat(value);
  return isNaN(num) ? 0 : num;
}

function formatAmount(num: number): string {
  return `$${num.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function LineItemTable({
  items,
  categories,
  isEditable,
  onAdd,
  onEdit,
  onDelete,
}: LineItemTableProps) {
  const categoryMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const cat of categories) {
      map.set(cat.id, cat.label);
    }
    return map;
  }, [categories]);

  const totals = useMemo(() => {
    let budgeted = 0;
    let actual = 0;
    let committed = 0;
    for (const item of items) {
      budgeted += parseDecimal(item.budgeted_amount);
      actual += parseDecimal(item.actual_amount);
      committed += parseDecimal(item.committed_amount);
    }
    return { budgeted, actual, committed };
  }, [items]);

  if (items.length === 0) {
    return (
      <div className="space-y-3">
        <div
          className="flex items-center justify-center rounded-lg border py-16 font-body"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-muted-foreground, #64748b)",
          }}
        >
          No line items yet.
        </div>
        {isEditable && onAdd && (
          <button
            type="button"
            onClick={onAdd}
            className="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-heading transition-colors"
            style={{
              backgroundColor: "var(--thittam-primary, #2563eb)",
              color: "#ffffff",
            }}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Line Item
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {/* Add button */}
      {isEditable && onAdd && (
        <div className="flex justify-end">
          <button
            type="button"
            onClick={onAdd}
            className="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-heading transition-colors"
            style={{
              backgroundColor: "var(--thittam-primary, #2563eb)",
              color: "#ffffff",
            }}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Line Item
          </button>
        </div>
      )}

      <div
        className="overflow-x-auto rounded-lg border"
        style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
      >
        <table className="w-full text-sm">
          <thead>
            <tr
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Category
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Description
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Account Code
              </th>
              <th
                className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Budgeted
              </th>
              <th
                className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Actual
              </th>
              <th
                className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Committed
              </th>
              <th
                className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Variance
              </th>
              {isEditable && (
                <th
                  className="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider font-heading"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Actions
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {items.map((item, idx) => (
              <tr
                key={item.id}
                className={idx % 2 === 1 ? "bg-gray-50/50" : ""}
                style={{
                  borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                <td
                  className="px-4 py-3 font-body"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {categoryMap.get(item.category_id) ?? item.category_id}
                </td>
                <td
                  className="px-4 py-3 font-body"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {item.description}
                </td>
                <td
                  className="px-4 py-3 font-mono tabular-nums"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {item.account_code}
                </td>
                <td
                  className="px-4 py-3 text-right font-mono tabular-nums"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {formatAmount(parseDecimal(item.budgeted_amount))}
                </td>
                <td
                  className="px-4 py-3 text-right font-mono tabular-nums"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {formatAmount(parseDecimal(item.actual_amount))}
                </td>
                <td
                  className="px-4 py-3 text-right font-mono tabular-nums"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {formatAmount(parseDecimal(item.committed_amount))}
                </td>
                <td className="px-4 py-3 text-right">
                  <VarianceIndicator
                    budgeted={item.budgeted_amount}
                    actual={item.actual_amount}
                    committed={item.committed_amount}
                  />
                </td>
                {isEditable && (
                  <td className="px-4 py-3 text-right">
                    <div className="inline-flex items-center gap-1">
                      {onEdit && !item.is_locked && (
                        <button
                          type="button"
                          onClick={() => onEdit(item)}
                          className="rounded p-1 transition-colors hover:bg-gray-100"
                          style={{
                            color: "var(--thittam-muted-foreground, #64748b)",
                          }}
                          aria-label={`Edit ${item.description}`}
                        >
                          <Pencil className="h-4 w-4" />
                        </button>
                      )}
                      {onDelete && !item.is_locked && (
                        <button
                          type="button"
                          onClick={() => onDelete(item.id)}
                          className="rounded p-1 transition-colors hover:bg-red-50 hover:text-red-600"
                          style={{
                            color: "var(--thittam-muted-foreground, #64748b)",
                          }}
                          aria-label={`Delete ${item.description}`}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      )}
                    </div>
                  </td>
                )}
              </tr>
            ))}
          </tbody>

          {/* Totals footer */}
          <tfoot>
            <tr
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                borderTop: "2px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <td
                colSpan={3}
                className="px-4 py-3 text-xs font-bold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Totals
              </td>
              <td
                className="px-4 py-3 text-right font-bold font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {formatAmount(totals.budgeted)}
              </td>
              <td
                className="px-4 py-3 text-right font-bold font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {formatAmount(totals.actual)}
              </td>
              <td
                className="px-4 py-3 text-right font-bold font-mono tabular-nums"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {formatAmount(totals.committed)}
              </td>
              <td className="px-4 py-3 text-right">
                <VarianceIndicator
                  budgeted={totals.budgeted.toFixed(2)}
                  actual={totals.actual.toFixed(2)}
                  committed={totals.committed.toFixed(2)}
                />
              </td>
              {isEditable && <td className="px-4 py-3" />}
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}
