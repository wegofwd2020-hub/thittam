"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Upload, Search } from "lucide-react";
import { toast } from "sonner";
import { useTheme } from "@/lib/themes/provider";
import { PageHeader } from "@/components/ui/page-header";
import { StatusBadge } from "@/components/ui/status-badge";
import { DataTable, type Column } from "@/components/ui/data-table";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type DocStatus = "draft" | "pending_signature" | "signed" | "archived";
type DocType =
  | "contract"
  | "permit"
  | "insurance"
  | "budget"
  | "agreement"
  | "nda"
  | "certificate";

interface DocumentRow {
  id: string;
  doc_number: string;
  name: string;
  doc_type: DocType;
  status: DocStatus;
  production: string;
  version: number;
  file_size_kb: number;
  mime_type: string;
  uploaded_by: string;
  uploaded_at: string;
  signatories_total: number;
  signatories_signed: number;
}

// ---------------------------------------------------------------------------
// Mock data — 8 documents for XYZ CBA Productions / The Last Horizon
// ---------------------------------------------------------------------------

const mockDocuments: DocumentRow[] = [
  {
    id: "doc-001",
    doc_number: "DOC-2026-001",
    name: "Budget Approval — The Last Horizon v3",
    doc_type: "budget",
    status: "signed",
    production: "The Last Horizon",
    version: 3,
    file_size_kb: 842,
    mime_type: "application/pdf",
    uploaded_by: "Deepa Nair",
    uploaded_at: "2026-02-14T11:00:00Z",
    signatories_total: 2,
    signatories_signed: 2,
  },
  {
    id: "doc-002",
    doc_number: "DOC-2026-002",
    name: "Film City Studio Location Agreement",
    doc_type: "agreement",
    status: "signed",
    production: "The Last Horizon",
    version: 1,
    file_size_kb: 312,
    mime_type: "application/pdf",
    uploaded_by: "Priya Sharma",
    uploaded_at: "2026-01-10T09:30:00Z",
    signatories_total: 2,
    signatories_signed: 2,
  },
  {
    id: "doc-003",
    doc_number: "DOC-2026-003",
    name: "Production Insurance Certificate — FY2026",
    doc_type: "insurance",
    status: "signed",
    production: "The Last Horizon",
    version: 1,
    file_size_kb: 198,
    mime_type: "application/pdf",
    uploaded_by: "Rajesh Kumar",
    uploaded_at: "2026-01-05T14:00:00Z",
    signatories_total: 1,
    signatories_signed: 1,
  },
  {
    id: "doc-004",
    doc_number: "DOC-2026-004",
    name: "Drone Operator Services Agreement — SkyVision Drones",
    doc_type: "contract",
    status: "pending_signature",
    production: "The Last Horizon",
    version: 1,
    file_size_kb: 256,
    mime_type: "application/pdf",
    uploaded_by: "Priya Sharma",
    uploaded_at: "2026-03-14T16:45:00Z",
    signatories_total: 2,
    signatories_signed: 1,
  },
  {
    id: "doc-005",
    doc_number: "DOC-2026-005",
    name: "Ladakh Location Permit Application",
    doc_type: "permit",
    status: "pending_signature",
    production: "The Last Horizon",
    version: 2,
    file_size_kb: 420,
    mime_type: "application/pdf",
    uploaded_by: "Priya Sharma",
    uploaded_at: "2026-03-20T10:00:00Z",
    signatories_total: 3,
    signatories_signed: 1,
  },
  {
    id: "doc-006",
    doc_number: "DOC-2026-006",
    name: "VFX Partner NDA — Pixelatom Studios",
    doc_type: "nda",
    status: "draft",
    production: "The Last Horizon",
    version: 1,
    file_size_kb: 145,
    mime_type: "application/pdf",
    uploaded_by: "Kiran Rao",
    uploaded_at: "2026-03-22T13:20:00Z",
    signatories_total: 2,
    signatories_signed: 0,
  },
  {
    id: "doc-007",
    doc_number: "DOC-2026-007",
    name: "Lead Actor Agreement — Vikram Anand",
    doc_type: "contract",
    status: "signed",
    production: "The Last Horizon",
    version: 2,
    file_size_kb: 614,
    mime_type: "application/pdf",
    uploaded_by: "Rajesh Kumar",
    uploaded_at: "2025-12-20T10:00:00Z",
    signatories_total: 3,
    signatories_signed: 3,
  },
  {
    id: "doc-008",
    doc_number: "DOC-2025-041",
    name: "Post Production Studio Booking Agreement",
    doc_type: "agreement",
    status: "archived",
    production: "Monsoon Chronicles",
    version: 1,
    file_size_kb: 280,
    mime_type: "application/pdf",
    uploaded_by: "Kiran Rao",
    uploaded_at: "2025-07-15T08:00:00Z",
    signatories_total: 2,
    signatories_signed: 2,
  },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

function formatFileSize(kb: number): string {
  if (kb < 1024) return `${kb} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

// ---------------------------------------------------------------------------
// Doc type badge
// ---------------------------------------------------------------------------

const DOC_TYPE_STYLES: Record<DocType, { bg: string; text: string; label: string }> = {
  contract:    { bg: "#dbeafe", text: "#1d4ed8", label: "Contract" },
  permit:      { bg: "#fef9c3", text: "#854d0e", label: "Permit" },
  insurance:   { bg: "#d1fae5", text: "#065f46", label: "Insurance" },
  budget:      { bg: "#ede9fe", text: "#6b21a8", label: "Budget" },
  agreement:   { bg: "#e0f2fe", text: "#0369a1", label: "Agreement" },
  nda:         { bg: "#fce7f3", text: "#9d174d", label: "NDA" },
  certificate: { bg: "#dcfce7", text: "#15803d", label: "Certificate" },
};

function DocTypeBadge({ type }: { type: DocType }) {
  const s = DOC_TYPE_STYLES[type];
  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
      style={{ backgroundColor: s.bg, color: s.text }}
    >
      {s.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Signature progress indicator
// ---------------------------------------------------------------------------

function SignatureProgress({
  signed,
  total,
}: {
  signed: number;
  total: number;
}) {
  if (total === 0) return <span style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>—</span>;
  const complete = signed === total;
  return (
    <span
      className="font-mono tabular-nums text-xs"
      style={{ color: complete ? "#065f46" : "var(--thittam-foreground, #0f172a)" }}
    >
      {signed}/{total}
      {complete && " ✓"}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Filter definitions
// ---------------------------------------------------------------------------

const STATUS_OPTIONS = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "pending_signature", label: "Pending Signature" },
  { key: "signed", label: "Signed" },
  { key: "archived", label: "Archived" },
] as const;

type StatusFilter = (typeof STATUS_OPTIONS)[number]["key"];

const TYPE_OPTIONS = [
  "All Types",
  ...Array.from(new Set(mockDocuments.map((d) => DOC_TYPE_STYLES[d.doc_type].label))),
];

const PRODUCTION_OPTIONS = [
  { id: "all", name: "All Productions" },
  ...Array.from(
    new Map(mockDocuments.map((d) => [d.production, { id: d.production, name: d.production }])).values()
  ),
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DocumentsPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [typeFilter, setTypeFilter] = useState("All Types");
  const [productionFilter, setProductionFilter] = useState("all");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    return mockDocuments.filter((d) => {
      if (statusFilter !== "all" && d.status !== statusFilter) return false;
      if (typeFilter !== "All Types" && DOC_TYPE_STYLES[d.doc_type].label !== typeFilter) return false;
      if (productionFilter !== "all" && d.production !== productionFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        if (!d.name.toLowerCase().includes(q) && !d.doc_number.toLowerCase().includes(q)) return false;
      }
      return true;
    });
  }, [statusFilter, typeFilter, productionFilter, search]);

  const columns: Column<DocumentRow>[] = [
    {
      key: "doc_number",
      header: "Doc #",
      sortable: true,
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.doc_number}
        </span>
      ),
    },
    {
      key: "name",
      header: "Name",
      sortable: true,
      render: (row) => (
        <div className="flex flex-col gap-1">
          <Link
            href={`/documents/${row.id}`}
            className="font-body text-sm font-medium hover:underline"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {row.name}
          </Link>
          <span
            className="font-mono tabular-nums text-[10px]"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            v{row.version} &middot; {formatFileSize(row.file_size_kb)}
          </span>
        </div>
      ),
    },
    {
      key: "doc_type",
      header: "Type",
      render: (row) => <DocTypeBadge type={row.doc_type} />,
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: "signatories_signed",
      header: "Signatures",
      type: "number",
      render: (row) => (
        <SignatureProgress signed={row.signatories_signed} total={row.signatories_total} />
      ),
    },
    {
      key: "production",
      header: `${entityLabels.project}`,
      sortable: true,
      render: (row) => (
        <span
          className="font-body text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.production}
        </span>
      ),
    },
    {
      key: "uploaded_by",
      header: "Uploaded By",
      render: (row) => (
        <span
          className="font-body text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.uploaded_by}
        </span>
      ),
    },
    {
      key: "uploaded_at",
      header: "Date",
      type: "date",
      sortable: true,
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {formatDate(row.uploaded_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <Link
          href={`/documents/${row.id}`}
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
        title="Documents"
        description={`Contracts, permits, and agreements across your ${entityLabels.projectPlural.toLowerCase()}`}
        icon="FileText"
        actions={
          <button
            onClick={() => toast.info("Upload dialog — drag-and-drop or browse to select a file")}
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Upload className="h-4 w-4" />
            Upload Document
          </button>
        }
      />

      {/* Filters row */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        {/* Status chips */}
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

        {/* Type, production, search */}
        <div className="flex flex-wrap items-center gap-3">
          <select
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            className="rounded-lg px-3 py-1.5 text-sm font-heading"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
            }}
          >
            {TYPE_OPTIONS.map((t) => (
              <option key={t} value={t}>
                {t}
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
                {p.name}
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
              placeholder="Search name or doc #"
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

      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={filtered as unknown as Record<string, unknown>[]}
        onRowClick={(row) =>
          router.push(`/documents/${(row as unknown as DocumentRow).id}`)
        }
        emptyMessage="No documents match the selected filters."
      />
    </div>
  );
}
