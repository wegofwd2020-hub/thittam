"use client";

import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getMonthlyCostBehavior,
  listLedgerAccounts,
  setAccountCostBehavior,
  type CostBehavior,
  type LedgerAccount,
  type MonthlyCostBehavior,
} from "@/lib/api/ledger";

// Palette from the validated reference instance (categorical slots 1 and 2 on
// the light surface, `#2a78d6` / `#eb6834`, both pass the CVD and contrast checks).
// Unclassified is neutral gray, not a categorical hue, so it reads as "unknown".
const SEGMENTS = [
  { key: "fixed", label: "Fixed", color: "#2a78d6" },
  { key: "variable", label: "Variable", color: "#eb6834" },
  { key: "unclassified", label: "Unclassified", color: "#8a8983" },
] as const;

const MONTHS_SHOWN = 12;

const accountsKey = ["ledger", "accounts"] as const;
const reportKey = ["ledger", "monthly-cost-behavior"] as const;

// Last MONTHS_SHOWN calendar months ending with the current one, as YYYY-MM.
function monthRange(now: Date): { from: string; to: string } {
  const fmt = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
  const from = new Date(now.getFullYear(), now.getMonth() - (MONTHS_SHOWN - 1), 1);
  return { from: fmt(from), to: fmt(now) };
}

function money(v: string): string {
  return Number(v).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

function monthLabel(ym: string): string {
  const [y, m] = ym.split("-").map(Number);
  return new Date(y, m - 1, 1).toLocaleString(undefined, {
    month: "short",
    year: "numeric",
  });
}

export function CostBehaviorReport() {
  const queryClient = useQueryClient();
  const range = useMemo(() => monthRange(new Date()), []);

  const reportQuery = useQuery({
    queryKey: [...reportKey, range.from, range.to],
    queryFn: () => getMonthlyCostBehavior(range.from, range.to),
  });

  const accountsQuery = useQuery({
    queryKey: accountsKey,
    queryFn: listLedgerAccounts,
  });

  const expenseAccounts = useMemo(
    () =>
      (accountsQuery.data?.accounts ?? [])
        .filter((a) => a.account_type === "expense" && a.is_active !== false)
        .sort((a, b) => a.code.localeCompare(b.code)),
    [accountsQuery.data],
  );

  const months: MonthlyCostBehavior[] = reportQuery.data?.months ?? [];
  // Bars scale to the busiest month so magnitude is comparable across rows.
  const maxTotal = Math.max(0, ...months.map((m) => Number(m.total)));

  const classify = useMutation({
    mutationFn: ({ id, value }: { id: string; value: CostBehavior }) =>
      setAccountCostBehavior(id, value),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: accountsKey });
      queryClient.invalidateQueries({ queryKey: reportKey });
    },
  });

  return (
    <div className="space-y-8">
      <section className="space-y-3">
        <div>
          <h2 className="text-base font-semibold">Fixed vs variable spend</h2>
          <p className="text-sm text-muted-foreground">
            Posted expense entries by posting month, last {MONTHS_SHOWN} months.
            Unclassified accounts are shown separately so totals reconcile.
          </p>
        </div>

        <ul className="flex flex-wrap gap-4 text-sm" aria-label="Legend">
          {SEGMENTS.map((s) => (
            <li key={s.key} className="flex items-center gap-2">
              <span
                aria-hidden="true"
                className="inline-block h-3 w-3 rounded-sm"
                style={{ backgroundColor: s.color }}
              />
              {s.label}
            </li>
          ))}
        </ul>

        {reportQuery.isLoading && (
          <p className="text-sm text-muted-foreground">Loading report…</p>
        )}
        {reportQuery.error && (
          <p role="alert" className="text-sm text-destructive">
            Failed to load report: {reportQuery.error.message}
          </p>
        )}

        {!reportQuery.isLoading && !reportQuery.error && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left">
                  <th scope="col" className="py-2 pr-4 font-medium">Month</th>
                  <th scope="col" className="py-2 pr-4 text-right font-medium">Fixed</th>
                  <th scope="col" className="py-2 pr-4 text-right font-medium">Variable</th>
                  <th scope="col" className="py-2 pr-4 text-right font-medium">Unclassified</th>
                  <th scope="col" className="py-2 pr-4 text-right font-medium">Total</th>
                  <th scope="col" className="py-2 font-medium">Mix</th>
                </tr>
              </thead>
              <tbody>
                {months.map((m) => {
                  const total = Number(m.total);
                  const widthPct = maxTotal > 0 ? (total / maxTotal) * 100 : 0;
                  return (
                    <tr key={m.month} className="border-b align-middle">
                      <th scope="row" className="py-2 pr-4 text-left font-normal">
                        {monthLabel(m.month)}
                      </th>
                      <td className="py-2 pr-4 text-right tabular-nums">{money(m.fixed)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{money(m.variable)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{money(m.unclassified)}</td>
                      <td className="py-2 pr-4 text-right font-medium tabular-nums">{money(m.total)}</td>
                      <td className="py-2 w-1/3">
                        {total > 0 ? (
                          <div
                            role="img"
                            aria-label={`${monthLabel(m.month)}: fixed ${money(m.fixed)}, variable ${money(m.variable)}, unclassified ${money(m.unclassified)}`}
                            className="flex h-4 overflow-hidden rounded-sm"
                            style={{ width: `${widthPct}%`, minWidth: "4px" }}
                          >
                            {SEGMENTS.map((s) => {
                              const v = Number(m[s.key]);
                              if (v <= 0) return null;
                              return (
                                <span
                                  key={s.key}
                                  style={{
                                    width: `${(v / total) * 100}%`,
                                    backgroundColor: s.color,
                                  }}
                                />
                              );
                            })}
                          </div>
                        ) : null}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="space-y-3">
        <div>
          <h2 className="text-base font-semibold">Classify expense accounts</h2>
          <p className="text-sm text-muted-foreground">
            Mark each expense account as fixed or variable. Changes apply to
            future reports; unclassified accounts stay visible in their own column.
          </p>
        </div>

        {classify.error && (
          <p role="alert" className="text-sm text-destructive">
            Could not save: {classify.error.message}
          </p>
        )}

        {accountsQuery.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading accounts…</p>
        ) : expenseAccounts.length === 0 ? (
          <p className="text-sm text-muted-foreground">No expense accounts yet.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-left">
                <th scope="col" className="py-2 pr-4 font-medium">Code</th>
                <th scope="col" className="py-2 pr-4 font-medium">Account</th>
                <th scope="col" className="py-2 font-medium">Cost behavior</th>
              </tr>
            </thead>
            <tbody>
              {expenseAccounts.map((a) => (
                <AccountRow
                  key={a.id}
                  account={a}
                  disabled={classify.isPending}
                  onChange={(value) => classify.mutate({ id: a.id, value })}
                />
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}

function AccountRow({
  account,
  disabled,
  onChange,
}: {
  account: LedgerAccount;
  disabled: boolean;
  onChange: (value: CostBehavior) => void;
}) {
  const id = `cost-behavior-${account.id}`;
  return (
    <tr className="border-b">
      <td className="py-2 pr-4 tabular-nums">{account.code}</td>
      <td className="py-2 pr-4">{account.name}</td>
      <td className="py-2">
        <label htmlFor={id} className="sr-only">
          Cost behavior for {account.name}
        </label>
        <select
          id={id}
          className="rounded-md border bg-background px-2 py-1"
          value={account.cost_behavior ?? ""}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value as CostBehavior)}
        >
          <option value="">Unclassified</option>
          <option value="fixed">Fixed</option>
          <option value="variable">Variable</option>
        </select>
      </td>
    </tr>
  );
}
