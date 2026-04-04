"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Plus, Search } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { PageHeader } from "@/components/ui/page-header";
import { AmountDisplay } from "@/components/ui/amount-display";
import { DataTable, type Column } from "@/components/ui/data-table";
import { AssetStatusBadge } from "@/components/inventory/asset-status-badge";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type AssetStatus = "available" | "checked_out" | "under_repair" | "retired";
type OwnershipType = "owned" | "rented" | "borrowed";

interface AssetRow {
  id: string;
  asset_code: string;
  name: string;
  category: string;
  category_id: string;
  status: AssetStatus;
  ownership_type: OwnershipType;
  serial_number: string;
  purchase_cost: string;
  currency: string;
}

// ---------------------------------------------------------------------------
// Mock data — 8 assets matching XYZ_CBA movie-production demo
// ---------------------------------------------------------------------------

const mockAssets: AssetRow[] = [
  {
    id: "asset-001",
    asset_code: "CAM-001",
    name: "ARRI Alexa Mini LF #1",
    category: "Camera Equipment",
    category_id: "cat-camera",
    status: "checked_out",
    ownership_type: "owned",
    serial_number: "ARRI-K1234567",
    purchase_cost: "4500000.00",
    currency: "INR",
  },
  {
    id: "asset-002",
    asset_code: "CAM-002",
    name: "ARRI Alexa Mini LF #2",
    category: "Camera Equipment",
    category_id: "cat-camera",
    status: "checked_out",
    ownership_type: "owned",
    serial_number: "ARRI-K1234568",
    purchase_cost: "4500000.00",
    currency: "INR",
  },
  {
    id: "asset-003",
    asset_code: "CAM-003",
    name: "Sony FX6",
    category: "Camera Equipment",
    category_id: "cat-camera",
    status: "available",
    ownership_type: "owned",
    serial_number: "SONY-FX6-98765",
    purchase_cost: "650000.00",
    currency: "INR",
  },
  {
    id: "asset-004",
    asset_code: "PRP-001",
    name: "Sci-fi control panels set",
    category: "Props",
    category_id: "cat-props",
    status: "checked_out",
    ownership_type: "owned",
    serial_number: "",
    purchase_cost: "180000.00",
    currency: "INR",
  },
  {
    id: "asset-005",
    asset_code: "PRP-002",
    name: "Period costumes collection",
    category: "Costumes & Wardrobe",
    category_id: "cat-costumes",
    status: "checked_out",
    ownership_type: "rented",
    serial_number: "",
    purchase_cost: "0.00",
    currency: "INR",
  },
  {
    id: "asset-006",
    asset_code: "PRP-003",
    name: "Breakaway furniture set",
    category: "Props",
    category_id: "cat-props",
    status: "available",
    ownership_type: "owned",
    serial_number: "",
    purchase_cost: "95000.00",
    currency: "INR",
  },
  {
    id: "asset-007",
    asset_code: "CAM-004",
    name: "DJI Inspire 3 Drone",
    category: "Camera Equipment",
    category_id: "cat-camera",
    status: "available",
    ownership_type: "owned",
    serial_number: "DJI-INS3-44556",
    purchase_cost: "320000.00",
    currency: "INR",
  },
  {
    id: "asset-008",
    asset_code: "CAM-005",
    name: "Movi Pro Gimbal",
    category: "Camera Equipment",
    category_id: "cat-camera",
    status: "available",
    ownership_type: "owned",
    serial_number: "MOVI-PRO-33221",
    purchase_cost: "275000.00",
    currency: "INR",
  },
];

// ---------------------------------------------------------------------------
// Filter chip definitions
// ---------------------------------------------------------------------------

const STATUS_OPTIONS = [
  { key: "all", label: "All" },
  { key: "available", label: "Available" },
  { key: "checked_out", label: "Checked Out" },
  { key: "under_repair", label: "Under Repair" },
  { key: "retired", label: "Retired" },
] as const;

type StatusKey = (typeof STATUS_OPTIONS)[number]["key"];

const CATEGORY_OPTIONS = [
  "All Categories",
  ...Array.from(new Set(mockAssets.map((a) => a.category))),
];

// ---------------------------------------------------------------------------
// Ownership badge
// ---------------------------------------------------------------------------

const OWNERSHIP_STYLES: Record<string, { bg: string; text: string }> = {
  owned: { bg: "bg-emerald-100", text: "text-emerald-700" },
  rented: { bg: "bg-purple-100", text: "text-purple-700" },
  borrowed: { bg: "bg-amber-100", text: "text-amber-700" },
};

function OwnershipBadge({ type }: { type: string }) {
  const style = OWNERSHIP_STYLES[type] ?? { bg: "bg-gray-100", text: "text-gray-600" };
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading capitalize ${style.bg} ${style.text}`}
    >
      {type}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function InventoryPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<StatusKey>("all");
  const [categoryFilter, setCategoryFilter] = useState("All Categories");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    return mockAssets.filter((a) => {
      if (statusFilter !== "all" && a.status !== statusFilter) return false;
      if (categoryFilter !== "All Categories" && a.category !== categoryFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        const matchName = a.name.toLowerCase().includes(q);
        const matchSerial = a.serial_number.toLowerCase().includes(q);
        if (!matchName && !matchSerial) return false;
      }
      return true;
    });
  }, [statusFilter, categoryFilter, search]);

  const columns: Column<AssetRow>[] = [
    {
      key: "asset_code",
      header: "Asset Code",
      sortable: true,
      render: (row) => (
        <span
          className="font-mono tabular-nums text-sm font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.asset_code}
        </span>
      ),
    },
    {
      key: "name",
      header: "Name",
      sortable: true,
      render: (row) => (
        <Link
          href={`/inventory/${row.id}`}
          className="font-heading text-sm font-medium hover:underline"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.name}
        </Link>
      ),
    },
    {
      key: "category",
      header: "Category",
      sortable: true,
      render: (row) => (
        <span
          className="font-heading text-xs font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.category}
        </span>
      ),
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <AssetStatusBadge status={row.status} />,
    },
    {
      key: "ownership_type",
      header: "Ownership",
      render: (row) => <OwnershipBadge type={row.ownership_type} />,
    },
    {
      key: "serial_number",
      header: "Serial #",
      render: (row) =>
        row.serial_number ? (
          <span
            className="font-mono tabular-nums text-xs"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {row.serial_number}
          </span>
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
      key: "purchase_cost",
      header: "Purchase Cost",
      type: "number",
      sortable: true,
      render: (row) =>
        row.purchase_cost !== "0.00" ? (
          <AmountDisplay amount={row.purchase_cost} currency={row.currency} size="sm" />
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
      key: "actions",
      header: "",
      render: (row) => (
        <Link
          href={`/inventory/${row.id}`}
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
        title="Inventory"
        description={`Track equipment, props, and assets across your ${entityLabels.projectPlural.toLowerCase()}`}
        icon="Package"
        actions={
          <Link
            href="/inventory/new"
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Plus className="h-4 w-4" />
            Add Asset
          </Link>
        }
      />

      {/* Filters row */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        {/* Status filter chips */}
        <div className="flex flex-wrap gap-2">
          {STATUS_OPTIONS.map((opt) => (
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

        {/* Category dropdown + search */}
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

          <div className="relative">
            <Search
              className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search name or serial #"
              className="rounded-lg py-1.5 pl-8 pr-3 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>
        </div>
      </div>

      {/* Asset table */}
      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={filtered as unknown as Record<string, unknown>[]}
        onRowClick={(row) => router.push(`/inventory/${(row as unknown as AssetRow).id}`)}
        emptyMessage="No assets match the selected filters."
      />
    </div>
  );
}
