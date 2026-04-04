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

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
interface Budget {
  id: string;
  label: string;
  productionId: string;
  productionTitle: string;
  status: "draft" | "submitted" | "approved" | "locked";
  totalAmount: string;
  currency: string;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// Mock data — matches XYZ_CBA seed pattern
// ---------------------------------------------------------------------------
const mockBudgets: Budget[] = [
  {
    id: "b1",
    label: "The Last Horizon V1",
    productionId: "1",
    productionTitle: "The Last Horizon",
    status: "approved",
    totalAmount: "85000000.00",
    currency: "INR",
    createdAt: "2026-01-20",
  },
  {
    id: "b2",
    label: "The Last Horizon V2",
    productionId: "1",
    productionTitle: "The Last Horizon",
    status: "draft",
    totalAmount: "92000000.00",
    currency: "INR",
    createdAt: "2026-03-10",
  },
  {
    id: "b3",
    label: "Midnight Express V1",
    productionId: "2",
    productionTitle: "Midnight Express Reboot",
    status: "locked",
    totalAmount: "120000000.00",
    currency: "INR",
    createdAt: "2025-06-15",
  },
  {
    id: "b4",
    label: "Midnight Express V2",
    productionId: "2",
    productionTitle: "Midnight Express Reboot",
    status: "submitted",
    totalAmount: "125000000.00",
    currency: "INR",
    createdAt: "2025-12-01",
  },
  {
    id: "b5",
    label: "Project Starfall V1",
    productionId: "3",
    productionTitle: "Project Starfall",
    status: "draft",
    totalAmount: "40000000.00",
    currency: "INR",
    createdAt: "2026-03-05",
  },
];

// ---------------------------------------------------------------------------
// Filter chip definitions
// ---------------------------------------------------------------------------
const FILTER_OPTIONS = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "submitted", label: "Submitted" },
  { key: "approved", label: "Approved" },
  { key: "locked", label: "Locked" },
] as const;

type FilterKey = (typeof FILTER_OPTIONS)[number]["key"];

// Unique productions for the filter dropdown
const PRODUCTION_OPTIONS = [
  { id: "all", title: "All Productions" },
  ...Array.from(
    new Map(
      mockBudgets.map((b) => [b.productionId, { id: b.productionId, title: b.productionTitle }])
    ).values()
  ),
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function BudgetsPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<FilterKey>("all");
  const [productionFilter, setProductionFilter] = useState("all");

  const filtered = useMemo(() => {
    return mockBudgets.filter((b) => {
      if (statusFilter !== "all" && b.status !== statusFilter) return false;
      if (productionFilter !== "all" && b.productionId !== productionFilter) return false;
      return true;
    });
  }, [statusFilter, productionFilter]);

  const columns: Column<Budget>[] = [
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

        {/* Production filter dropdown */}
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
          {PRODUCTION_OPTIONS.map((p) => (
            <option key={p.id} value={p.id}>
              {p.title}
            </option>
          ))}
        </select>
      </div>

      {/* Budget table */}
      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={filtered as unknown as Record<string, unknown>[]}
        onRowClick={(row) => router.push(`/budgets/${(row as unknown as Budget).id}`)}
        emptyMessage="No budgets match the selected filters."
      />
    </div>
  );
}
