"use client";

import { useState } from "react";
import Link from "next/link";
import { Plus, ArrowLeft } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { AmountDisplay } from "@/components/ui/amount-display";
import { DataTable, type Column } from "@/components/ui/data-table";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type POStatus = "draft" | "sent" | "acknowledged" | "approved" | "closed";

interface PurchaseOrder {
  id: string;
  poNumber: string;
  vendor: string;
  description: string;
  amount: string;
  currency: string;
  gstin: string;
  status: POStatus;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// PO Status Pipeline
// ---------------------------------------------------------------------------
const PO_STAGES: POStatus[] = ["draft", "sent", "acknowledged", "approved", "closed"];
const PO_STAGE_LABELS: Record<POStatus, string> = {
  draft: "Draft",
  sent: "Sent",
  acknowledged: "Ack",
  approved: "Approved",
  closed: "Closed",
};

function POStatusPipeline({ currentStatus }: { currentStatus: POStatus }) {
  const currentIdx = PO_STAGES.indexOf(currentStatus);
  return (
    <div className="flex items-center gap-1">
      {PO_STAGES.map((stage, idx) => (
        <div key={stage} className="flex items-center gap-1">
          <div
            className="flex h-5 items-center rounded-full px-2 text-[9px] font-heading font-medium"
            style={
              idx <= currentIdx
                ? {
                    backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 15%, transparent)",
                    color: "var(--thittam-primary, #3b82f6)",
                  }
                : {
                    backgroundColor: "var(--thittam-muted, #f1f5f9)",
                    color: "var(--thittam-muted-foreground, #64748b)",
                  }
            }
          >
            {PO_STAGE_LABELS[stage]}
          </div>
          {idx < PO_STAGES.length - 1 && (
            <div
              className="h-0.5 w-2"
              style={{
                backgroundColor:
                  idx < currentIdx
                    ? "var(--thittam-primary, #3b82f6)"
                    : "var(--thittam-border, #e2e8f0)",
              }}
            />
          )}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Mock data — 5 POs
// ---------------------------------------------------------------------------
const mockPOs: PurchaseOrder[] = [
  {
    id: "po-001",
    poNumber: "PO-001",
    vendor: "Film City Studios",
    description: "Studio rental — 45 shooting days, Stage 4 & 7",
    amount: "1800000.00",
    currency: "INR",
    gstin: "27AABCF1234C1Z5",
    status: "approved",
    createdAt: "2026-01-10",
  },
  {
    id: "po-002",
    poNumber: "PO-002",
    vendor: "Goa Locations Pvt Ltd",
    description: "Location permits & logistics — 3 sites in North Goa",
    amount: "500000.00",
    currency: "INR",
    gstin: "30AABCG5678D1Z8",
    status: "approved",
    createdAt: "2026-01-20",
  },
  {
    id: "po-003",
    poNumber: "PO-003",
    vendor: "ARRI Rental India",
    description: "ARRI Alexa Mini LF camera package — 60 day rental",
    amount: "2200000.00",
    currency: "INR",
    gstin: "29AABCA9012E1Z3",
    status: "approved",
    createdAt: "2026-01-25",
  },
  {
    id: "po-004",
    poNumber: "PO-004",
    vendor: "Heritage Building Trust",
    description: "Heritage site shoot permission — 5 days with restricted hours",
    amount: "250000.00",
    currency: "INR",
    gstin: "27AABCH3456F1Z1",
    status: "draft",
    createdAt: "2026-03-10",
  },
  {
    id: "po-005",
    poNumber: "PO-005",
    vendor: "SkyView Drone Services",
    description: "Drone operator & equipment — aerial shots package",
    amount: "180000.00",
    currency: "INR",
    gstin: "29AABCS7890G1Z6",
    status: "draft",
    createdAt: "2026-03-12",
  },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function PurchaseOrdersPage() {
  const [data] = useState(mockPOs);

  const columns: Column<PurchaseOrder>[] = [
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
      render: (row) => <POStatusPipeline currentStatus={row.status} />,
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
        data={data as unknown as Record<string, unknown>[]}
        emptyMessage="No purchase orders found."
      />
    </div>
  );
}
