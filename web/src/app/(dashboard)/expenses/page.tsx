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
// Mock data — 12 expenses matching XYZ_CBA pattern
// ---------------------------------------------------------------------------
const mockExpenses: Expense[] = [
  {
    id: "exp-001",
    description: "Film City Studio Rental — 45 days",
    category: "Locations",
    taxTreatment: "gst_18",
    amount: "1800000.00",
    taxAmount: "324000.00",
    currency: "INR",
    status: "paid",
    poNumber: "PO-001",
    poId: "po-001",
    submittedBy: "Ravi Kumar",
    date: "2026-01-15",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-002",
    description: "Goa Location Permits & Logistics",
    category: "Locations",
    taxTreatment: "gst_18",
    amount: "500000.00",
    taxAmount: "90000.00",
    currency: "INR",
    status: "paid",
    poNumber: "PO-002",
    poId: "po-002",
    submittedBy: "Priya Sharma",
    date: "2026-01-28",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-003",
    description: "ARRI Alexa Mini LF Camera Package",
    category: "Camera Equipment",
    taxTreatment: "gst_18",
    amount: "2200000.00",
    taxAmount: "396000.00",
    currency: "INR",
    status: "paid",
    poNumber: "PO-003",
    poId: "po-003",
    submittedBy: "Arjun Mehta",
    date: "2026-02-05",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-004",
    description: "Lighting Equipment — Tungsten + LED Kit",
    category: "Camera Equipment",
    taxTreatment: "gst_18",
    amount: "800000.00",
    taxAmount: "144000.00",
    currency: "INR",
    status: "paid",
    poNumber: null,
    poId: null,
    submittedBy: "Arjun Mehta",
    date: "2026-02-10",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-005",
    description: "Ladakh Location Recce & Permits",
    category: "Locations",
    taxTreatment: "gst_5",
    amount: "750000.00",
    taxAmount: "37500.00",
    currency: "INR",
    status: "approved",
    poNumber: null,
    poId: null,
    submittedBy: "Priya Sharma",
    date: "2026-03-01",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-006",
    description: "Luxury Hotel — Cast Accommodation (at approval limit)",
    category: "Travel & Accommodation",
    taxTreatment: "gst_18",
    amount: "200000.00",
    taxAmount: "36000.00",
    currency: "INR",
    status: "approved",
    poNumber: null,
    poId: null,
    submittedBy: "Deepa Nair",
    date: "2026-03-05",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-007",
    description: "Heritage Building Shoot Location",
    category: "Locations",
    taxTreatment: "gst_12",
    amount: "250000.00",
    taxAmount: "30000.00",
    currency: "INR",
    status: "submitted",
    poNumber: "PO-004",
    poId: "po-004",
    submittedBy: "Priya Sharma",
    date: "2026-03-12",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-008",
    description: "Drone Operator — Aerial Shots Package",
    category: "Camera Equipment",
    taxTreatment: "tds_194j",
    amount: "180000.00",
    taxAmount: "18000.00",
    currency: "INR",
    status: "submitted",
    poNumber: "PO-005",
    poId: "po-005",
    submittedBy: "Arjun Mehta",
    date: "2026-03-15",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-009",
    description: "VFX Render Farm — 2000 hrs",
    category: "Post Production",
    taxTreatment: "gst_18",
    amount: "150000.00",
    taxAmount: "27000.00",
    currency: "INR",
    status: "submitted",
    poNumber: null,
    poId: null,
    submittedBy: "Kiran Rao",
    date: "2026-03-20",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-010",
    description: "IMAX Camera Rental — 30 days",
    category: "Camera Equipment",
    taxTreatment: "gst_18",
    amount: "3500000.00",
    taxAmount: "630000.00",
    currency: "INR",
    status: "rejected",
    poNumber: null,
    poId: null,
    submittedBy: "Arjun Mehta",
    date: "2026-03-08",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-011",
    description: "Post Production Studio Booking — 3 weeks",
    category: "Post Production",
    taxTreatment: "gst_18",
    amount: "400000.00",
    taxAmount: "72000.00",
    currency: "INR",
    status: "draft",
    poNumber: null,
    poId: null,
    submittedBy: "Kiran Rao",
    date: "2026-03-25",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: false,
  },
  {
    id: "exp-012",
    description: "Crew Meals — Outdoor Shoot (3 days)",
    category: "Crew & Catering",
    taxTreatment: "gst_5",
    amount: "8500.00",
    taxAmount: "425.00",
    currency: "INR",
    status: "paid",
    poNumber: null,
    poId: null,
    submittedBy: "Deepa Nair",
    date: "2026-02-20",
    productionId: "1",
    productionTitle: "The Last Horizon",
    isPettyCash: true,
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
  { key: "rejected", label: "Rejected" },
  { key: "paid", label: "Paid" },
] as const;

type FilterKey = (typeof FILTER_OPTIONS)[number]["key"];

const CATEGORY_OPTIONS = [
  "All Categories",
  "Locations",
  "Camera Equipment",
  "Post Production",
  "Travel & Accommodation",
  "Crew & Catering",
];

const PRODUCTION_OPTIONS = [
  { id: "all", title: "All Productions" },
  ...Array.from(
    new Map(
      mockExpenses.map((e) => [e.productionId, { id: e.productionId, title: e.productionTitle }])
    ).values()
  ),
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function ExpensesPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<FilterKey>("all");
  const [categoryFilter, setCategoryFilter] = useState("All Categories");
  const [productionFilter, setProductionFilter] = useState("all");

  const filtered = useMemo(() => {
    return mockExpenses.filter((e) => {
      if (statusFilter !== "all" && e.status !== statusFilter) return false;
      if (categoryFilter !== "All Categories" && e.category !== categoryFilter) return false;
      if (productionFilter !== "all" && e.productionId !== productionFilter) return false;
      return true;
    });
  }, [statusFilter, categoryFilter, productionFilter]);

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
            {CATEGORY_OPTIONS.map((c) => (
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
            {PRODUCTION_OPTIONS.map((p) => (
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
        onRowClick={(row) => router.push(`/expenses/${(row as unknown as Expense).id}`)}
        emptyMessage="No expenses match the selected filters."
      />
    </div>
  );
}
