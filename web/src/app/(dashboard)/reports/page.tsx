"use client";

import { useTheme } from "@/lib/themes/provider";
import { useReportDefinitions } from "@/lib/hooks/use-reports";
import { PageHeader } from "@/components/ui/page-header";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import Link from "next/link";
import { BarChart3, FileText } from "lucide-react";

// ---------------------------------------------------------------------------
// Icon picker — choose contextual icon based on report id
// ---------------------------------------------------------------------------

const REPORT_ICONS: Record<string, typeof BarChart3> = {
  cost_report: BarChart3,
  burn_rate: BarChart3,
  project_pnl: BarChart3,
  event_pnl: BarChart3,
  boq_variance: BarChart3,
};

export default function ReportsPage() {
  const { entityLabels, theme } = useTheme();
  const verticalId = theme.id;
  const { data: reports, isLoading } = useReportDefinitions(verticalId);

  return (
    <div className="mx-auto max-w-5xl">
      <PageHeader
        title="Reports"
        description={`Cross-service analytics and reports for your ${entityLabels.projectPlural.toLowerCase()}.`}
        icon="BarChart3"
      />

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      ) : !reports || reports.length === 0 ? (
        <div
          className="flex items-center justify-center rounded-lg border py-16 font-body"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-muted-foreground, #64748b)",
          }}
        >
          No report definitions available for this vertical.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {reports.map((report) => {
            const Icon = REPORT_ICONS[report.id] ?? FileText;

            return (
              <Link
                key={report.id}
                href={`/reports/${report.id}`}
                className="group flex flex-col gap-3 rounded-xl p-5 transition-shadow hover:shadow-md"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                <div className="flex items-center gap-3">
                  <div
                    className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
                    style={{
                      backgroundColor:
                        "color-mix(in srgb, var(--thittam-primary, #3b82f6) 10%, transparent)",
                      color: "var(--thittam-primary, #3b82f6)",
                    }}
                  >
                    <Icon className="h-5 w-5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <h2
                      className="text-sm font-semibold font-heading"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {report.name}
                    </h2>
                    <p
                      className="mt-0.5 text-xs font-body line-clamp-2"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      {report.description}
                    </p>
                  </div>
                </div>

                {/* Data source tags */}
                <div className="flex flex-wrap gap-1.5">
                  {report.data_sources.map((src) => (
                    <span
                      key={src}
                      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-mono"
                      style={{
                        backgroundColor: "var(--thittam-muted, #f1f5f9)",
                        color: "var(--thittam-muted-foreground, #64748b)",
                      }}
                    >
                      {src}
                    </span>
                  ))}
                </div>

                {/* View Report link */}
                <div className="mt-auto pt-1">
                  <span
                    className="text-xs font-heading font-semibold group-hover:underline"
                    style={{ color: "var(--thittam-primary, #3b82f6)" }}
                  >
                    View Report &rarr;
                  </span>
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
