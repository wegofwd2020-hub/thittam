"use client";

import { useMemo } from "react";
import { DataTable, type Column } from "@/components/ui/data-table";
import { StatusBadge } from "@/components/ui/status-badge";

// ---------------------------------------------------------------------------
// ComplianceTable — table of compliance violations with severity badges.
// ---------------------------------------------------------------------------

export interface ComplianceItem {
  id: string;
  type: string;
  description: string;
  severity: "warning" | "critical";
  detectedAt: string;
  projectTitle: string;
}

interface ComplianceTableProps {
  items: ComplianceItem[];
  isLoading?: boolean;
}

const SEVERITY_STATUS: Record<string, string> = {
  warning: "at_risk",
  critical: "over_budget",
};

export function ComplianceTable({ items, isLoading = false }: ComplianceTableProps) {
  const columns = useMemo<Column<ComplianceItem & Record<string, unknown>>[]>(
    () => [
      {
        key: "type",
        header: "Type",
        type: "text",
        sortable: true,
        render: (row) => (
          <span className="font-heading text-xs font-medium uppercase tracking-wide">
            {row.type}
          </span>
        ),
      },
      {
        key: "description",
        header: "Description",
        type: "text",
        render: (row) => (
          <span className="font-body text-sm">{row.description}</span>
        ),
      },
      {
        key: "severity",
        header: "Severity",
        render: (row) => (
          <StatusBadge
            status={SEVERITY_STATUS[row.severity] ?? "at_risk"}
            variant="health"
          />
        ),
      },
      {
        key: "projectTitle",
        header: "Project",
        type: "text",
        sortable: true,
        render: (row) => (
          <span
            className="font-body text-sm underline-offset-2 hover:underline"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            {row.projectTitle}
          </span>
        ),
      },
      {
        key: "detectedAt",
        header: "Detected",
        type: "date",
        sortable: true,
        render: (row) => (
          <span className="font-mono tabular-nums text-xs">
            {row.detectedAt}
          </span>
        ),
      },
    ],
    [],
  );

  // DataTable expects Record<string, unknown> — widen the type
  const data = items as (ComplianceItem & Record<string, unknown>)[];

  return (
    <DataTable
      columns={columns}
      data={data}
      isLoading={isLoading}
      emptyMessage="No compliance violations detected."
    />
  );
}
