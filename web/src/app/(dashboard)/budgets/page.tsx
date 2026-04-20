"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueries } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { PageHeader } from "@/components/ui/page-header";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import { DataTable, type Column } from "@/components/ui/data-table";
import { useProductions } from "@/lib/hooks/use-productions";
import { budgetKeys } from "@/lib/hooks/use-budgets";
import { listBudgets, type Budget } from "@/lib/api/budgets";

interface BudgetRow {
  id: string;
  label: string;
  productionId: string;
  productionTitle: string;
  status: string;
  totalAmount: string;
  currency: string;
  createdAt: string;
}

const FILTER_OPTIONS = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "submitted", label: "Submitted" },
  { key: "approved", label: "Approved" },
  { key: "locked", label: "Locked" },
] as const;

type FilterKey = (typeof FILTER_OPTIONS)[number]["key"];

export default function BudgetsPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<FilterKey>("all");
  const [productionFilter, setProductionFilter] = useState("all");

  const productionsQuery = useProductions();
  const productions = productionsQuery.data?.productions ?? [];

  // Fan out: one useBudgets query per production. There is no tenant-wide
  // list endpoint (#60 follow-up); the per-production endpoint is what the
  // backend exposes today.
  const budgetQueries = useQueries({
    queries: productions.map((p) => ({
      queryKey: budgetKeys.list(p.id),
      queryFn: () => listBudgets(p.id),
      enabled: !!p.id,
    })),
  });

  const rows: BudgetRow[] = useMemo(() => {
    const byId = new Map(productions.map((p) => [p.id, p.title]));
    const flat: BudgetRow[] = [];
    for (const q of budgetQueries) {
      const budgets = (q.data?.budgets ?? []) as Budget[];
      for (const b of budgets) {
        flat.push({
          id: b.id,
          label: b.label,
          productionId: b.production_id,
          productionTitle: byId.get(b.production_id) ?? "—",
          status: b.status,
          totalAmount: b.total_amount,
          currency: b.currency,
          createdAt: b.created_at.slice(0, 10),
        });
      }
    }
    return flat;
  }, [productions, budgetQueries]);

  const filtered = useMemo(() => {
    return rows.filter((b) => {
      if (statusFilter !== "all" && b.status !== statusFilter) return false;
      if (productionFilter !== "all" && b.productionId !== productionFilter) return false;
      return true;
    });
  }, [rows, statusFilter, productionFilter]);

  const productionOptions = useMemo(
    () => [
      { id: "all", title: "All Productions" },
      ...productions.map((p) => ({ id: p.id, title: p.title })),
    ],
    [productions],
  );

  const isLoading =
    productionsQuery.isLoading || budgetQueries.some((q) => q.isLoading);
  const errored = productionsQuery.error ?? budgetQueries.find((q) => q.error)?.error;

  const columns: Column<BudgetRow>[] = [
    {
      key: "label",
      header: "Label",
      sortable: true,
      render: (row) => (
        <Link
          href={`/budgets/${row.id}`}
          className="font-heading text-sm font-medium hover:underline"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          {row.label}
        </Link>
      ),
    },
    {
      key: "productionTitle",
      header: entityLabels.project,
      sortable: true,
      render: (row) => (
        <span
          className="font-body text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.productionTitle}
        </span>
      ),
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <StatusBadge status={row.status} variant="budget" />,
    },
    {
      key: "totalAmount",
      header: "Total Amount",
      type: "number",
      sortable: true,
      render: (row) => (
        <AmountDisplay amount={row.totalAmount} currency={row.currency} size="sm" />
      ),
    },
    {
      key: "createdAt",
      header: "Created",
      type: "date",
      sortable: true,
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <Link
          href={`/budgets/${row.id}`}
          className="text-xs font-heading font-medium transition-colors hover:opacity-80"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          View
        </Link>
      ),
    },
  ];

  const emptyMessage = errored
    ? `Failed to load budgets: ${errored.message}`
    : isLoading
      ? "Loading budgets…"
      : "No budgets match the selected filters.";

  return (
    <div className="mx-auto max-w-6xl">
      <PageHeader
        title="Budgets"
        description={`Manage budget versions across your ${entityLabels.projectPlural.toLowerCase()}`}
        icon="Wallet"
        actions={
          <Link
            href="/budgets/new"
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Plus className="h-4 w-4" />
            New Budget
          </Link>
        }
      />

      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-2">
          {FILTER_OPTIONS.map((opt) => (
            <button
              key={opt.key}
              onClick={() => setStatusFilter(opt.key)}
              className={`rounded-full px-3 py-1 text-xs font-heading font-medium transition-colors ${
                statusFilter === opt.key ? "text-white" : ""
              }`}
              style={
                statusFilter === opt.key
                  ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
                  : {
                      backgroundColor: "var(--thittam-muted, #f1f5f9)",
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }
              }
            >
              {opt.label}
            </button>
          ))}
        </div>

        <select
          value={productionFilter}
          onChange={(e) => setProductionFilter(e.target.value)}
          className="rounded-lg px-3 py-1.5 text-sm font-heading"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
          }}
        >
          {productionOptions.map((p) => (
            <option key={p.id} value={p.id}>
              {p.title}
            </option>
          ))}
        </select>
      </div>

      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={filtered as unknown as Record<string, unknown>[]}
        onRowClick={(row) => router.push(`/budgets/${(row as unknown as BudgetRow).id}`)}
        emptyMessage={emptyMessage}
      />
    </div>
  );
}
