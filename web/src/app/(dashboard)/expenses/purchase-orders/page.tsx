"use client";

import Link from "next/link";
import { Plus, ArrowLeft } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { AmountDisplay } from "@/components/ui/amount-display";
import { StatusBadge } from "@/components/ui/status-badge";
import { DataTable, type Column } from "@/components/ui/data-table";
import { usePurchaseOrders } from "@/lib/hooks/use-expenses";
import type { PurchaseOrder as ApiPurchaseOrder } from "@/lib/api/expenses";

// ---------------------------------------------------------------------------
// Row shape used by the table (mapped from the API's snake_case fields)
// ---------------------------------------------------------------------------
interface PurchaseOrderRow {
  id: string;
  poNumber: string;
  vendor: string;
  description: string;
  amount: string;
  currency: string;
  gstin: string;
  status: string;
  createdAt: string;
}

function toRow(po: ApiPurchaseOrder): PurchaseOrderRow {
  return {
    id: po.id,
    poNumber: po.po_number,
    vendor: po.vendor_name,
    description: po.description,
    amount: po.amount,
    currency: po.currency,
    gstin: po.vendor_gstin,
    status: po.status,
    createdAt: (po.raised_at ?? "").slice(0, 10),
  };
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function PurchaseOrdersPage() {
  const posQuery = usePurchaseOrders();
  const rows = (posQuery.data?.purchase_orders ?? []).map(toRow);

  const columns: Column<PurchaseOrderRow>[] = [
    {
      key: "poNumber",
      header: "PO Number",
      sortable: true,
      render: (row) => (
        <span
          className="font-mono tabular-nums text-sm font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          {row.poNumber}
        </span>
      ),
    },
    {
      key: "vendor",
      header: "Vendor",
      sortable: true,
      render: (row) => (
        <span
          className="font-heading text-sm font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.vendor}
        </span>
      ),
    },
    {
      key: "description",
      header: "Description",
      render: (row) => (
        <span
          className="font-body text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.description}
        </span>
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
      key: "status",
      header: "Status",
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: "gstin",
      header: "GSTIN",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.gstin}
        </span>
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
      render: () => (
        <span
          className="text-xs font-heading font-medium transition-colors hover:opacity-80 cursor-pointer"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          View
        </span>
      ),
    },
  ];

  return (
    <div className="mx-auto max-w-7xl">
      {/* Back link */}
      <Link
        href="/expenses"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Expenses
      </Link>

      <PageHeader
        title="Purchase Orders"
        description="Manage purchase orders for vendor contracts and service agreements"
        icon="FileText"
        actions={
          <button
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Plus className="h-4 w-4" />
            Create PO
          </button>
        }
      />

      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={rows as unknown as Record<string, unknown>[]}
        isLoading={posQuery.isLoading}
        emptyMessage={
          posQuery.error
            ? `Failed to load purchase orders: ${posQuery.error.message}`
            : "No purchase orders found."
        }
      />
    </div>
  );
}
