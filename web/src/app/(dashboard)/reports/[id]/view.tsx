"use client";

import { useParams } from "next/navigation";
import { useCallback } from "react";
import { useReportDefinition, useReportData } from "@/lib/hooks/use-reports";
import { PageHeader } from "@/components/ui/page-header";
import { DataTable, type Column } from "@/components/ui/data-table";
import { PageSkeleton } from "@/components/ui/loading-skeleton";
import Link from "next/link";
import { ArrowLeft, Download } from "lucide-react";

// ---------------------------------------------------------------------------
// Columns that should render in font-mono (amounts, percentages, dates).
// ---------------------------------------------------------------------------

const NUMERIC_KEYWORDS = [
  "budget", "actual", "variance", "amount", "cost", "revenue",
  "margin", "paid", "balance", "value", "rate", "qty", "retention",
  "certified", "completed", "held", "planned", "cumulative",
  "total", "head",
];
const PERCENT_KEYWORDS = ["% used", "utilisation", "margin %", "% of total"];
const DATE_KEYWORDS = ["date", "detected"];

function inferColumnType(header: string): "number" | "date" | "text" {
  const lower = header.toLowerCase();
  if (PERCENT_KEYWORDS.some((kw) => lower.includes(kw))) return "number";
  if (DATE_KEYWORDS.some((kw) => lower.includes(kw))) return "date";
  if (NUMERIC_KEYWORDS.some((kw) => lower.includes(kw))) return "number";
  return "text";
}

// ---------------------------------------------------------------------------
// CSV export helper
// ---------------------------------------------------------------------------

function downloadCsv(columns: string[], rows: Record<string, string>[]) {
  const header = columns.join(",");
  const body = rows
    .map((row) =>
      columns
        .map((col) => {
          const val = row[col] ?? "";
          // Wrap in quotes if the value contains commas
          return val.includes(",") ? `"${val}"` : val;
        })
        .join(","),
    )
    .join("\n");

  const blob = new Blob([`${header}\n${body}`], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "report.csv";
  a.click();
  URL.revokeObjectURL(url);
}

// ---------------------------------------------------------------------------
// Report Viewer Page
// ---------------------------------------------------------------------------

export function ReportDetailView() {
  const params = useParams<{ id: string }>();
  const reportId = params.id;

  const { data: definition, isLoading: defLoading } =
    useReportDefinition(reportId);
  const { data: reportData, isLoading: dataLoading } = useReportData(
    reportId,
    {},
  );

  const handleExport = useCallback(() => {
    if (!reportData) return;
    downloadCsv(reportData.columns, reportData.rows);
  }, [reportData]);

  if (defLoading) {
    return (
      <div className="mx-auto max-w-6xl">
        <PageSkeleton />
      </div>
    );
  }

  if (!definition) {
    return (
      <div className="mx-auto max-w-6xl">
        <div
          className="flex flex-col items-center justify-center gap-4 rounded-lg border py-16 font-body"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-muted-foreground, #64748b)",
          }}
        >
          <p>Report not found.</p>
          <Link
            href="/reports"
            className="text-sm font-heading font-semibold"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            &larr; Back to Reports
          </Link>
        </div>
      </div>
    );
  }

  // Build DataTable columns from the report definition
  const columns: Column<Record<string, unknown>>[] =
    definition.default_columns.map((col) => ({
      key: col,
      header: col,
      type: inferColumnType(col),
      sortable: true,
      render: (row: Record<string, unknown>) => {
        const type = inferColumnType(col);
        const value = (row[col] as string) ?? "\u2014";

        if (type === "number" || type === "date") {
          return (
            <span className="font-mono tabular-nums text-sm">{value}</span>
          );
        }
        return <span className="font-body text-sm">{value}</span>;
      },
    }));

  const rows = (reportData?.rows ?? []) as Record<string, unknown>[];

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      {/* Back link */}
      <Link
        href="/reports"
        className="inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Reports
      </Link>

      {/* Page header with export action */}
      <PageHeader
        title={definition.name}
        description={definition.description}
        icon="BarChart3"
        actions={
          <button
            type="button"
            onClick={handleExport}
            disabled={!reportData || reportData.rows.length === 0}
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-semibold font-heading transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            style={{
              backgroundColor: "var(--thittam-primary, #3b82f6)",
              color: "var(--thittam-primary-foreground, #fff)",
            }}
          >
            <Download className="h-4 w-4" />
            Export CSV
          </button>
        }
      />

      {/* Data source tags */}
      <div className="flex flex-wrap gap-1.5">
        {definition.data_sources.map((src) => (
          <span
            key={src}
            className="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium font-mono"
            style={{
              backgroundColor: "var(--thittam-muted, #f1f5f9)",
              color: "var(--thittam-muted-foreground, #64748b)",
            }}
          >
            {src}
          </span>
        ))}
      </div>

      {/* Data table */}
      <DataTable
        columns={columns}
        data={rows}
        isLoading={dataLoading}
        emptyMessage="No data available for this report."
      />
    </div>
  );
}
