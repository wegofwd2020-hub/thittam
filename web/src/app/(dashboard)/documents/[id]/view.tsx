"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Download,
  FilePlus,
  Send,
  Archive,
  CheckCircle2,
  Clock,
  FileText,
} from "lucide-react";
import { toast } from "sonner";
import { StatusBadge } from "@/components/ui/status-badge";

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
type SignatoryStatus = "pending" | "signed" | "declined";

interface DocumentVersion {
  version: number;
  uploaded_by: string;
  uploaded_at: string;
  file_size_kb: number;
  change_note: string;
}

interface Signatory {
  id: string;
  name: string;
  email: string;
  role: string;
  status: SignatoryStatus;
  signed_at: string | null;
}

interface DocumentDetail {
  id: string;
  doc_number: string;
  name: string;
  doc_type: DocType;
  status: DocStatus;
  production: string;
  production_id: string;
  current_version: number;
  file_size_kb: number;
  mime_type: string;
  storage_ref: string;
  uploaded_by: string;
  uploaded_at: string;
  description: string;
  versions: DocumentVersion[];
  signatories: Signatory[];
}

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const mockDocuments: Record<string, DocumentDetail> = {
  "doc-001": {
    id: "doc-001",
    doc_number: "DOC-2026-001",
    name: "Budget Approval — The Last Horizon v3",
    doc_type: "budget",
    status: "signed",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 3,
    file_size_kb: 842,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-001/v3.pdf",
    uploaded_by: "Deepa Nair",
    uploaded_at: "2026-02-14T11:00:00Z",
    description:
      "Executive-approved budget for The Last Horizon, version 3. Reflects the revised Ladakh location costs and updated VFX allocation following the March steering committee.",
    versions: [
      {
        version: 3,
        uploaded_by: "Deepa Nair",
        uploaded_at: "2026-02-14T11:00:00Z",
        file_size_kb: 842,
        change_note: "Revised Ladakh location costs (+₹7.5L) and VFX allocation (+₹5L). Approved by EP.",
      },
      {
        version: 2,
        uploaded_by: "Deepa Nair",
        uploaded_at: "2026-01-28T14:30:00Z",
        file_size_kb: 810,
        change_note: "Updated camera equipment rental line — ARRI package confirmed.",
      },
      {
        version: 1,
        uploaded_by: "Arjun Mehta",
        uploaded_at: "2026-01-15T09:00:00Z",
        file_size_kb: 788,
        change_note: "Initial budget draft submitted for EP review.",
      },
    ],
    signatories: [
      {
        id: "sig-001",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "Super Admin / Authorising Signatory",
        status: "signed",
        signed_at: "2026-02-15T09:20:00Z",
      },
      {
        id: "sig-002",
        name: "Priya Sharma",
        email: "priya.sharma@xyzcba.com",
        role: "Executive Producer",
        status: "signed",
        signed_at: "2026-02-14T17:45:00Z",
      },
    ],
  },
  "doc-002": {
    id: "doc-002",
    doc_number: "DOC-2026-002",
    name: "Film City Studio Location Agreement",
    doc_type: "agreement",
    status: "signed",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 1,
    file_size_kb: 312,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-002/v1.pdf",
    uploaded_by: "Priya Sharma",
    uploaded_at: "2026-01-10T09:30:00Z",
    description:
      "Location usage agreement for Film City Studio Stage 4 & 7 covering 45 shooting days from 15 Jan to 31 Mar 2026.",
    versions: [
      {
        version: 1,
        uploaded_by: "Priya Sharma",
        uploaded_at: "2026-01-10T09:30:00Z",
        file_size_kb: 312,
        change_note: "Final version signed by both parties.",
      },
    ],
    signatories: [
      {
        id: "sig-003",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "XYZ CBA Productions",
        status: "signed",
        signed_at: "2026-01-10T10:15:00Z",
      },
      {
        id: "sig-004",
        name: "Film City Authority",
        email: "contracts@filmcity.co.in",
        role: "Film City Mumbai",
        status: "signed",
        signed_at: "2026-01-09T16:00:00Z",
      },
    ],
  },
  "doc-003": {
    id: "doc-003",
    doc_number: "DOC-2026-003",
    name: "Production Insurance Certificate — FY2026",
    doc_type: "insurance",
    status: "signed",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 1,
    file_size_kb: 198,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-003/v1.pdf",
    uploaded_by: "Rajesh Kumar",
    uploaded_at: "2026-01-05T14:00:00Z",
    description:
      "Comprehensive production insurance certificate covering equipment, cast, crew, and third-party liability for FY2026 productions. Issued by National Insurance Co. Ltd.",
    versions: [
      {
        version: 1,
        uploaded_by: "Rajesh Kumar",
        uploaded_at: "2026-01-05T14:00:00Z",
        file_size_kb: 198,
        change_note: "Certificate received from insurer. Valid 01-Apr-2025 to 31-Mar-2026.",
      },
    ],
    signatories: [
      {
        id: "sig-005",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "Policy Holder",
        status: "signed",
        signed_at: "2026-01-05T14:30:00Z",
      },
    ],
  },
  "doc-004": {
    id: "doc-004",
    doc_number: "DOC-2026-004",
    name: "Drone Operator Services Agreement — SkyVision Drones",
    doc_type: "contract",
    status: "pending_signature",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 1,
    file_size_kb: 256,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-004/v1.pdf",
    uploaded_by: "Priya Sharma",
    uploaded_at: "2026-03-14T16:45:00Z",
    description:
      "Services agreement with SkyVision Drones for aerial cinematography package. Covers 5 shoot days at Ladakh with DGCA-certified operators and Zenmuse X9 payload.",
    versions: [
      {
        version: 1,
        uploaded_by: "Priya Sharma",
        uploaded_at: "2026-03-14T16:45:00Z",
        file_size_kb: 256,
        change_note: "Initial draft sent to SkyVision for review and signature.",
      },
    ],
    signatories: [
      {
        id: "sig-006",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "XYZ CBA Productions",
        status: "signed",
        signed_at: "2026-03-15T10:00:00Z",
      },
      {
        id: "sig-007",
        name: "Arjun Verma",
        email: "contracts@skyvisiondrones.com",
        role: "SkyVision Drones",
        status: "pending",
        signed_at: null,
      },
    ],
  },
  "doc-005": {
    id: "doc-005",
    doc_number: "DOC-2026-005",
    name: "Ladakh Location Permit Application",
    doc_type: "permit",
    status: "pending_signature",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 2,
    file_size_kb: 420,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-005/v2.pdf",
    uploaded_by: "Priya Sharma",
    uploaded_at: "2026-03-20T10:00:00Z",
    description:
      "Official permit application to the District Collector, Leh for film shooting at Pangong Lake and Nubra Valley. Revised to include drone operation schedule.",
    versions: [
      {
        version: 2,
        uploaded_by: "Priya Sharma",
        uploaded_at: "2026-03-20T10:00:00Z",
        file_size_kb: 420,
        change_note: "Added drone operation schedule and revised shooting dates per DC office feedback.",
      },
      {
        version: 1,
        uploaded_by: "Priya Sharma",
        uploaded_at: "2026-03-05T11:00:00Z",
        file_size_kb: 375,
        change_note: "Initial permit application submitted.",
      },
    ],
    signatories: [
      {
        id: "sig-008",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "XYZ CBA Productions (Applicant)",
        status: "signed",
        signed_at: "2026-03-20T11:00:00Z",
      },
      {
        id: "sig-009",
        name: "DC Office, Leh",
        email: "dcoffice@leh.nic.in",
        role: "Granting Authority",
        status: "pending",
        signed_at: null,
      },
      {
        id: "sig-010",
        name: "Tourism Dept., J&K",
        email: "tourism@jk.gov.in",
        role: "Co-Signatory (Tourism)",
        status: "pending",
        signed_at: null,
      },
    ],
  },
  "doc-006": {
    id: "doc-006",
    doc_number: "DOC-2026-006",
    name: "VFX Partner NDA — Pixelatom Studios",
    doc_type: "nda",
    status: "draft",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 1,
    file_size_kb: 145,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-006/v1.pdf",
    uploaded_by: "Kiran Rao",
    uploaded_at: "2026-03-22T13:20:00Z",
    description:
      "Mutual NDA with Pixelatom Studios covering VFX brief, unreleased footage, and production timeline. Pending legal review before circulation.",
    versions: [
      {
        version: 1,
        uploaded_by: "Kiran Rao",
        uploaded_at: "2026-03-22T13:20:00Z",
        file_size_kb: 145,
        change_note: "Initial draft. Awaiting legal review.",
      },
    ],
    signatories: [
      {
        id: "sig-011",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "XYZ CBA Productions",
        status: "pending",
        signed_at: null,
      },
      {
        id: "sig-012",
        name: "Pixelatom Studios",
        email: "legal@pixelatom.in",
        role: "VFX Partner",
        status: "pending",
        signed_at: null,
      },
    ],
  },
  "doc-007": {
    id: "doc-007",
    doc_number: "DOC-2026-007",
    name: "Lead Actor Agreement — Vikram Anand",
    doc_type: "contract",
    status: "signed",
    production: "The Last Horizon",
    production_id: "prod-001",
    current_version: 2,
    file_size_kb: 614,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-007/v2.pdf",
    uploaded_by: "Rajesh Kumar",
    uploaded_at: "2025-12-20T10:00:00Z",
    description:
      "Principal cast agreement for Vikram Anand in the lead role of 'Aryan'. Includes remuneration, call sheet obligations, approvals, and social media restrictions.",
    versions: [
      {
        version: 2,
        uploaded_by: "Rajesh Kumar",
        uploaded_at: "2025-12-20T10:00:00Z",
        file_size_kb: 614,
        change_note: "Revised remuneration schedule and added streaming residuals clause.",
      },
      {
        version: 1,
        uploaded_by: "Rajesh Kumar",
        uploaded_at: "2025-12-05T15:00:00Z",
        file_size_kb: 590,
        change_note: "Initial agreement draft shared with talent agency.",
      },
    ],
    signatories: [
      {
        id: "sig-013",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "XYZ CBA Productions",
        status: "signed",
        signed_at: "2025-12-20T11:00:00Z",
      },
      {
        id: "sig-014",
        name: "Vikram Anand",
        email: "vikram@startalent.in",
        role: "Principal Cast",
        status: "signed",
        signed_at: "2025-12-19T14:30:00Z",
      },
      {
        id: "sig-015",
        name: "Star Talent Agency",
        email: "contracts@startalent.in",
        role: "Talent Representative",
        status: "signed",
        signed_at: "2025-12-19T14:00:00Z",
      },
    ],
  },
  "doc-008": {
    id: "doc-008",
    doc_number: "DOC-2025-041",
    name: "Post Production Studio Booking Agreement",
    doc_type: "agreement",
    status: "archived",
    production: "Monsoon Chronicles",
    production_id: "prod-002",
    current_version: 1,
    file_size_kb: 280,
    mime_type: "application/pdf",
    storage_ref: "tenant_xyz/documents/doc-008/v1.pdf",
    uploaded_by: "Kiran Rao",
    uploaded_at: "2025-07-15T08:00:00Z",
    description:
      "Post production studio booking for Monsoon Chronicles. Archived after production wrap.",
    versions: [
      {
        version: 1,
        uploaded_by: "Kiran Rao",
        uploaded_at: "2025-07-15T08:00:00Z",
        file_size_kb: 280,
        change_note: "Signed agreement. Archived post production wrap Dec 2025.",
      },
    ],
    signatories: [
      {
        id: "sig-016",
        name: "Rajesh Kumar",
        email: "rajesh.kumar@xyzcba.com",
        role: "XYZ CBA Productions",
        status: "signed",
        signed_at: "2025-07-15T10:00:00Z",
      },
      {
        id: "sig-017",
        name: "FrameWorks Post Studio",
        email: "bookings@frameworkspost.com",
        role: "Studio",
        status: "signed",
        signed_at: "2025-07-14T16:00:00Z",
      },
    ],
  },
};

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

function formatDateTime(iso: string | null): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function formatFileSize(kb: number): string {
  if (kb < 1024) return `${kb} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

// ---------------------------------------------------------------------------
// Doc type badge
// ---------------------------------------------------------------------------

const DOC_TYPE_STYLES: Record<
  DocType,
  { bg: string; text: string; label: string }
> = {
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
      className="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium font-heading"
      style={{ backgroundColor: s.bg, color: s.text }}
    >
      {s.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Version history timeline
// ---------------------------------------------------------------------------

function VersionTimeline({ versions }: { versions: DocumentVersion[] }) {
  return (
    <ol className="relative ml-3 border-l" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
      {versions.map((v, idx) => (
        <li key={v.version} className="mb-6 ml-6 last:mb-0">
          {/* Dot */}
          <span
            className="absolute -left-2.5 flex h-5 w-5 items-center justify-center rounded-full ring-4 ring-white"
            style={{
              backgroundColor:
                idx === 0
                  ? "var(--thittam-primary, #3b82f6)"
                  : "var(--thittam-muted, #f1f5f9)",
            }}
          >
            <FileText
              className="h-2.5 w-2.5"
              style={{ color: idx === 0 ? "#fff" : "var(--thittam-muted-foreground, #64748b)" }}
            />
          </span>

          {/* Content */}
          <div className="flex flex-col gap-0.5">
            <div className="flex items-center gap-2">
              <span
                className="font-heading text-sm font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Version {v.version}
              </span>
              {idx === 0 && (
                <span
                  className="rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
                  style={{
                    backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 12%, transparent)",
                    color: "var(--thittam-primary, #3b82f6)",
                  }}
                >
                  Current
                </span>
              )}
              <span
                className="font-mono tabular-nums text-xs"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                {formatFileSize(v.file_size_kb)}
              </span>
            </div>
            <p
              className="text-xs font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {v.uploaded_by} &middot;{" "}
              <span className="font-mono tabular-nums">{formatDate(v.uploaded_at)}</span>
            </p>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {v.change_note}
            </p>
            <button
              onClick={() => toast.success(`Downloading v${v.version}.pdf`)}
              className="mt-1 inline-flex w-fit items-center gap-1 text-xs font-heading font-medium transition-opacity hover:opacity-70"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            >
              <Download className="h-3 w-3" />
              Download v{v.version}
            </button>
          </div>
        </li>
      ))}
    </ol>
  );
}

// ---------------------------------------------------------------------------
// E-signature panel
// ---------------------------------------------------------------------------

function SignatoryRow({ sig, onResend }: { sig: Signatory; onResend: () => void }) {
  const isSigned = sig.status === "signed";
  const isDeclined = sig.status === "declined";

  return (
    <div
      className="flex items-start justify-between gap-4 rounded-lg px-4 py-3"
      style={{
        backgroundColor: isSigned
          ? "rgba(209, 250, 229, 0.3)"
          : isDeclined
            ? "rgba(254, 226, 226, 0.3)"
            : "var(--thittam-muted, #f8fafc)",
        border: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      {/* Left */}
      <div className="flex items-start gap-3">
        <div className="mt-0.5">
          {isSigned ? (
            <CheckCircle2 className="h-5 w-5 text-emerald-500" />
          ) : isDeclined ? (
            <CheckCircle2 className="h-5 w-5 text-red-400" />
          ) : (
            <Clock className="h-5 w-5" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
          )}
        </div>
        <div>
          <span
            className="block font-heading text-sm font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {sig.name}
          </span>
          <span
            className="font-mono tabular-nums text-xs"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {sig.email}
          </span>
          <span
            className="mt-0.5 block text-xs font-body"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {sig.role}
          </span>
          {isSigned && sig.signed_at && (
            <span
              className="mt-0.5 block font-mono tabular-nums text-xs text-emerald-600"
            >
              Signed {formatDateTime(sig.signed_at)}
            </span>
          )}
        </div>
      </div>

      {/* Right */}
      <div className="flex shrink-0 items-center gap-2">
        {!isSigned && !isDeclined && (
          <button
            onClick={onResend}
            className="inline-flex items-center gap-1 rounded px-2.5 py-1 text-xs font-heading font-medium transition-opacity hover:opacity-80"
            style={{
              backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 10%, transparent)",
              color: "var(--thittam-primary, #3b82f6)",
            }}
          >
            <Send className="h-3 w-3" />
            Resend
          </button>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function DocumentDetailView() {
  const params = useParams<{ id: string }>();
  const [doc, setDoc] = useState<DocumentDetail | null>(
    () => mockDocuments[params.id] ?? null,
  );

  if (!doc) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <h1
          className="font-heading text-xl font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Document not found
        </h1>
        <p
          className="mt-2 font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          The document you are looking for does not exist or has been removed.
        </p>
        <Link
          href="/documents"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Documents
        </Link>
      </div>
    );
  }

  const pendingCount = doc.signatories.filter((s) => s.status === "pending").length;

  function handleResend(sigId: string) {
    const sig = doc!.signatories.find((s) => s.id === sigId);
    toast.success(`Signature reminder sent to ${sig?.name}`);
  }

  function handleArchive() {
    if (!window.confirm(`Archive "${doc!.name}"? The document will be read-only.`)) return;
    setDoc((prev) => (prev ? { ...prev, status: "archived" } : prev));
    toast.success("Document archived");
  }

  const isArchived = doc.status === "archived";

  return (
    <div className="mx-auto max-w-5xl">
      {/* Back link */}
      <Link
        href="/documents"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Documents
      </Link>

      {/* Header */}
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <span
              className="font-mono tabular-nums text-xs"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {doc.doc_number}
            </span>
            <DocTypeBadge type={doc.doc_type} />
            <StatusBadge status={doc.status} />
          </div>
          <h1
            className="mt-2 font-heading text-2xl font-semibold tracking-tight"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {doc.name}
          </h1>
        </div>

        {/* Action buttons */}
        {!isArchived && (
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <button
              onClick={() => toast.success(`Downloading v${doc.current_version}.pdf`)}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium transition-opacity hover:opacity-80"
              style={{
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
                backgroundColor: "var(--thittam-background, #fff)",
              }}
            >
              <Download className="h-4 w-4" />
              Download
            </button>
            <button
              onClick={() => toast.info("Upload new version dialog")}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium transition-opacity hover:opacity-80"
              style={{
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
                backgroundColor: "var(--thittam-background, #fff)",
              }}
            >
              <FilePlus className="h-4 w-4" />
              New Version
            </button>
            {doc.status === "draft" && (
              <button
                onClick={() => toast.info("Send for signature — opens signatory selection")}
                className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium text-white transition-opacity hover:opacity-90"
                style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
              >
                <Send className="h-4 w-4" />
                Request Signatures
              </button>
            )}
            <button
              onClick={handleArchive}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium transition-opacity hover:opacity-80 text-amber-600"
              style={{
                border: "1px solid #fde68a",
                backgroundColor: "#fffbeb",
              }}
            >
              <Archive className="h-4 w-4" />
              Archive
            </button>
          </div>
        )}
      </div>

      {/* ------------------------------------------------------------------ */}
      {/* Metadata card                                                       */}
      {/* ------------------------------------------------------------------ */}
      <div
        className="mb-6 rounded-xl p-5"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Production
            </span>
            <span
              className="mt-1 block font-body text-sm font-medium"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            >
              {doc.production}
            </span>
          </div>

          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Document Type
            </span>
            <div className="mt-1">
              <DocTypeBadge type={doc.doc_type} />
            </div>
          </div>

          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Current Version
            </span>
            <span
              className="mt-1 block font-mono tabular-nums text-sm"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              v{doc.current_version}
            </span>
          </div>

          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              File Size
            </span>
            <span
              className="mt-1 block font-mono tabular-nums text-sm"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {formatFileSize(doc.file_size_kb)}
            </span>
          </div>

          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              MIME Type
            </span>
            <span
              className="mt-1 block font-mono tabular-nums text-xs"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {doc.mime_type}
            </span>
          </div>

          <div>
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Uploaded By
            </span>
            <span
              className="mt-1 block font-body text-sm"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {doc.uploaded_by}
            </span>
          </div>

          <div className="sm:col-span-2">
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Storage Reference
            </span>
            <span
              className="mt-1 block font-mono tabular-nums text-xs"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {doc.storage_ref}
            </span>
          </div>
        </div>

        {doc.description && (
          <div
            className="mt-5 border-t pt-4"
            style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
          >
            <span
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Description
            </span>
            <p
              className="mt-1 text-sm font-body leading-relaxed"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              {doc.description}
            </p>
          </div>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* ---------------------------------------------------------------- */}
        {/* Version history                                                  */}
        {/* ---------------------------------------------------------------- */}
        <div
          className="rounded-xl p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <h2
            className="mb-5 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Version History
          </h2>
          <VersionTimeline versions={doc.versions} />
        </div>

        {/* ---------------------------------------------------------------- */}
        {/* E-signature panel                                                */}
        {/* ---------------------------------------------------------------- */}
        <div
          className="rounded-xl p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <div className="mb-4 flex items-center justify-between">
            <h2
              className="font-heading text-sm font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Signatures
            </h2>
            <span
              className="font-mono tabular-nums text-xs"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {doc.signatories.filter((s) => s.status === "signed").length}/
              {doc.signatories.length} signed
            </span>
          </div>

          {pendingCount > 0 && (
            <div
              className="mb-4 rounded-lg px-3 py-2 text-xs font-body"
              style={{ backgroundColor: "#eff6ff", color: "#1d4ed8" }}
            >
              {pendingCount} signature{pendingCount > 1 ? "s" : ""} outstanding
              — use Resend to send a reminder.
            </div>
          )}

          <div className="flex flex-col gap-2">
            {doc.signatories.map((sig) => (
              <SignatoryRow
                key={sig.id}
                sig={sig}
                onResend={() => handleResend(sig.id)}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
