"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Plus } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { PageHeader } from "@/components/ui/page-header";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import { DataTable, type Column } from "@/components/ui/data-table";
import type { Expense as ApiExpense, ExpenseCategory } from "@/lib/api/expenses";
import { useExpenses, useExpenseCategories } from "@/lib/hooks/use-expenses";
import { useProductions } from "@/lib/hooks/use-productions";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type ExpenseStatus = "draft" | "submitted" | "approved" | "rejected" | "paid";
type TaxTreatment = "gst_18" | "gst_12" | "gst_5" | "gst_exempt" | "tds_194c" | "tds_194j";

interface Expense {
  id: string;
  description: string;
  category: string;
  taxTreatment: TaxTreatment;
  amount: string;
  taxAmount: string;
  currency: string;
  status: ExpenseStatus;
  poNumber: string | null;
  poId: string | null;
  submittedBy: string;
  date: string;
  productionId: string;
  productionTitle: string;
  isPettyCash: boolean;
}

// ---------------------------------------------------------------------------
// Tax treatment display config
// ---------------------------------------------------------------------------
const TAX_LABELS: Record<TaxTreatment, { label: string; color: string; bg: string }> = {
  gst_18: { label: "GST 18%", color: "#9333ea", bg: "#f3e8ff" },
  gst_12: { label: "GST 12%", color: "#7c3aed", bg: "#ede9fe" },
  gst_5: { label: "GST 5%", color: "#6d28d9", bg: "#ede9fe" },
  gst_exempt: { label: "GST Exempt", color: "#64748b", bg: "#f1f5f9" },
  tds_194c: { label: "TDS 194C", color: "#0369a1", bg: "#e0f2fe" },
  tds_194j: { label: "TDS 194J", color: "#0e7490", bg: "#cffafe" },
};

function TaxTreatmentBadge({ treatment }: { treatment: TaxTreatment }) {
  const config = TAX_LABELS[treatment];
  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
      style={{ backgroundColor: config.bg, color: config.color }}
    >
      {config.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Filter chip definitions
// ---------------------------------------------------------------------------
const FILTER_OPTIONS = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "submitted", label: "Submitted" },
  { key: "approved", label: "Approved" },
  { key: "rejected", label: "Rejected" },
  { key: "paid", label: "Paid" },
] as const;

type FilterKey = (typeof FILTER_OPTIONS)[number]["key"];


// ---------------------------------------------------------------------------
// API → row mapping
// ---------------------------------------------------------------------------
function toTaxTreatment(value: string | undefined): TaxTreatment {
  return value && value in TAX_LABELS ? (value as TaxTreatment) : "gst_exempt";
}

function toRow(
  e: ApiExpense,
  categories: Map<string, ExpenseCategory>,
  productions: Map<string, string>,
): Expense {
  const cat = categories.get(e.category_id);
  return {
    id: e.id,
    description: e.description,
    category: cat?.label ?? "Uncategorised",
    taxTreatment: toTaxTreatment(cat?.tax_treatment),
    amount: e.amount,
    taxAmount: e.tax_amount,
    currency: e.currency,
    status: e.status as ExpenseStatus,
    poNumber: e.purchase_order_id || null,
    poId: e.purchase_order_id || null,
    submittedBy: e.submitted_by ?? "",
    date: (e.submitted_at ?? e.created_at ?? "").slice(0, 10),
    productionId: e.production_id,
    productionTitle: productions.get(e.production_id) ?? e.production_id,
    isPettyCash: false,
  };
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function ExpensesPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<FilterKey>("all");
  const [categoryFilter, setCategoryFilter] = useState("All Categories");
  const [productionFilter, setProductionFilter] = useState("all");

  const expensesQuery = useExpenses();
  const productionsQuery = useProductions();
  const categoriesQuery = useExpenseCategories();

  const productionOptions = useMemo(
    () => [
      { id: "all", title: "All Productions" },
      ...(productionsQuery.data?.productions ?? []).map((p) => ({ id: p.id, title: p.title })),
    ],
    [productionsQuery.data],
  );

  const rows = useMemo(() => {
    const cats = new Map((categoriesQuery.data?.categories ?? []).map((c) => [c.id, c]));
    const prods = new Map((productionsQuery.data?.productions ?? []).map((p) => [p.id, p.title]));
    return (expensesQuery.data?.expenses ?? []).map((e) => toRow(e, cats, prods));
  }, [expensesQuery.data, categoriesQuery.data, productionsQuery.data]);

  const categoryOptions = useMemo(
    () => ["All Categories", ...Array.from(new Set(rows.map((r) => r.category)))],
    [rows],
  );

  const filtered = useMemo(() => {
    return rows.filter((e) => {
      if (statusFilter !== "all" && e.status !== statusFilter) return false;
      if (categoryFilter !== "All Categories" && e.category !== categoryFilter) return false;
      if (productionFilter !== "all" && e.productionId !== productionFilter) return false;
      return true;
    });
  }, [rows, statusFilter, categoryFilter, productionFilter]);

  const columns: Column<Expense>[] = [
    {
      key: "description",
      header: "Description",
      sortable: true,
      render: (row) => (
        <div className="flex flex-col gap-0.5">
          <Link
            href={`/expenses/${row.id}`}
            className="font-body text-sm font-medium hover:underline"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {row.description}
          </Link>
          {row.isPettyCash && (
            <span
              className="inline-flex w-fit items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
              style={{ backgroundColor: "#fef3c7", color: "#92400e" }}
            >
              Petty Cash
            </span>
          )}
        </div>
      ),
    },
    {
      key: "category",
      header: "Category",
      sortable: true,
      render: (row) => (
        <div className="flex flex-col gap-1">
          <span
            className="font-heading text-xs font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {row.category}
          </span>
          <TaxTreatmentBadge treatment={row.taxTreatment} />
        </div>
      ),
    },
    {
      key: "amount",
      header: "Amount",
      type: "number",
      sortable: true,
      render: (row) => (
        <AmountDisplay amount={row.amount} currency={row.currency} size="sm" />
      ),
    },
    {
      key: "taxAmount",
      header: "Tax",
      type: "number",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          <AmountDisplay amount={row.taxAmount} currency={row.currency} size="sm" />
        </span>
      ),
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: "poNumber",
      header: "PO #",
      render: (row) =>
        row.poNumber ? (
          <Link
            href={`/expenses/purchase-orders`}
            className="font-mono tabular-nums text-xs hover:underline"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            {row.poNumber}
          </Link>
        ) : (
          <span
            className="font-mono text-xs"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {"\u2014"}
          </span>
        ),
    },
    {
      key: "submittedBy",
      header: "Submitted By",
      render: (row) => (
        <span
          className="font-body text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.submittedBy}
        </span>
      ),
    },
    {
      key: "date",
      header: "Date",
      type: "date",
      sortable: true,
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <Link
          href={`/expenses/${row.id}`}
          className="text-xs font-heading font-medium transition-colors hover:opacity-80"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          View
        </Link>
      ),
    },
  ];

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title="Expenses"
        description={`Track and manage expenses across your ${entityLabels.projectPlural.toLowerCase()}`}
        icon="Receipt"
        actions={
          <Link
            href="/expenses/new"
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Plus className="h-4 w-4" />
            Submit Expense
          </Link>
        }
      />

      {/* Filters row */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        {/* Status filter chips */}
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

        {/* Category + Production dropdowns */}
        <div className="flex items-center gap-3">
          <select
            value={categoryFilter}
            onChange={(e) => setCategoryFilter(e.target.value)}
            className="rounded-lg px-3 py-1.5 text-sm font-heading"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
            }}
          >
            {categoryOptions.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>

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
      </div>

      {/* Quick links */}
      <div className="mb-4 flex items-center gap-4">
        <Link
          href="/expenses/purchase-orders"
          className="text-xs font-heading font-medium transition-colors hover:opacity-80"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          Purchase Orders
        </Link>
        <Link
          href="/expenses/petty-cash"
          className="text-xs font-heading font-medium transition-colors hover:opacity-80"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          Petty Cash
        </Link>
      </div>

      {/* Expense table */}
      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={filtered as unknown as Record<string, unknown>[]}
        isLoading={expensesQuery.isLoading}
        onRowClick={(row) => router.push(`/expenses/${(row as unknown as Expense).id}`)}
        emptyMessage={
          expensesQuery.error
            ? `Failed to load expenses: ${expensesQuery.error.message}`
            : "No expenses match the selected filters."
        }
      />
    </div>
  );
}
