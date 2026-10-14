"use client";

import { useAuth } from "@/lib/auth/context";
import { useTheme } from "@/lib/themes/provider";
import {
  useApprovalPipeline,
  useComplianceStatus,
  useFinancialSummary,
  usePortfolioOverview,
  useTeamUtilization,
} from "@/lib/hooks/use-dashboard";
import { KPICard } from "@/components/dashboard/kpi-card";
import { BudgetHealthPie } from "@/components/dashboard/budget-health-pie";
import { BurnRateChart } from "@/components/dashboard/burn-rate-chart";
import { ApprovalAgeBars } from "@/components/dashboard/approval-age-bars";
import { UtilizationGauge } from "@/components/dashboard/utilization-gauge";
import { ComplianceTable, type ComplianceItem } from "@/components/dashboard/compliance-table";
import { ActivityTimeline } from "@/components/dashboard/activity-timeline";
import Link from "next/link";
import {
  FolderKanban,
  Wallet,
  TrendingUp,
  Clock,
  AlertTriangle,
  ShieldAlert,
  Flag,
  Plus,
  ArrowRight,
  CheckCircle2,
} from "lucide-react";

// Compact display for totals (e.g. 1.2M). Amounts arrive as decimal strings
// from the reporting service; Number() here is for display only, never for
// arithmetic that is stored or compared.
const compact = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

function formatTotal(amount: string | undefined): string {
  if (amount === undefined) return "—";
  return compact.format(Number(amount));
}

// Percentage change between the last two months of the burn-rate trend, or
// null when there is not enough history to compare.
function monthOverMonthPct(
  trend: { actual: string }[] | undefined,
): string | null {
  if (!trend || trend.length < 2) return null;
  const prev = Number(trend[trend.length - 2].actual);
  const last = Number(trend[trend.length - 1].actual);
  if (prev === 0) return null;
  return (((last - prev) / prev) * 100).toFixed(1);
}

// ---------------------------------------------------------------------------
// Helper components
// ---------------------------------------------------------------------------

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h2
      className="mb-4 text-lg font-semibold tracking-tight font-heading"
      style={{ color: "var(--thittam-foreground, #0f172a)" }}
    >
      {children}
    </h2>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export default function DashboardPage() {
  const { user } = useAuth();
  const { entityLabels } = useTheme();

  const portfolioQuery = usePortfolioOverview();
  const financialQuery = useFinancialSummary();
  const approvalsQuery = useApprovalPipeline();
  const teamQuery = useTeamUtilization();
  const complianceQuery = useComplianceStatus();

  const portfolio = portfolioQuery.data;
  const financial = financialQuery.data;
  const approvals = approvalsQuery.data;
  const team = teamQuery.data;
  const compliance = complianceQuery.data;

  const failedSections = [
    portfolioQuery,
    financialQuery,
    approvalsQuery,
    teamQuery,
    complianceQuery,
  ].filter((q) => q.isError).length;

  const totalBudget = portfolio?.total_budget;
  const totalActual = portfolio?.total_actual;
  const budgetUsedPct =
    totalBudget && Number(totalBudget) > 0 && totalActual !== undefined
      ? ((Number(totalActual) / Number(totalBudget)) * 100).toFixed(1)
      : null;
  const spendTrendPct = monthOverMonthPct(financial?.monthly_trend);
  const pendingApprovals = approvals?.total_pending;

  const complianceItems: ComplianceItem[] = (compliance?.recent_violations ?? []).map(
    (v) => ({
      id: v.id,
      type: v.type,
      description: v.description,
      severity: v.severity === "critical" ? "critical" : "warning",
      detectedAt: v.detected_at,
      projectTitle: v.project_title,
    }),
  );

  const burnRateData = (financial?.monthly_trend ?? []).map((m) => ({
    month: m.month,
    actual: Number(m.actual),
    budget: Number(m.budget),
  }));

  return (
    <div className="mx-auto max-w-7xl space-y-8">
      {/* Greeting */}
      <div>
        <h1
          className="text-2xl font-bold tracking-tight font-heading"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Welcome{user ? `, ${user.display_name}` : ""}
        </h1>
        <p
          className="mt-1 text-sm font-body"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Executive overview of your {entityLabels.projectPlural.toLowerCase()}.
        </p>
        {failedSections > 0 && (
          <p className="mt-2 text-sm text-red-700">
            {failedSections === 1
              ? "One dashboard section failed to load."
              : `${failedSections} dashboard sections failed to load.`}
          </p>
        )}
      </div>

      {/* ── Row 1: KPI Cards ────────────────────────────────────────── */}
      <section>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <KPICard
            title={`Total ${entityLabels.projectPlural}`}
            value={portfolio?.total_active ?? "—"}
            subtitle="Active"
            icon={<FolderKanban className="h-4 w-4" />}
          />

          <KPICard
            title="Total Budget"
            value={formatTotal(totalBudget)}
            subtitle="Approved allocation"
            icon={<Wallet className="h-4 w-4" />}
          />

          <KPICard
            title="Total Actual Spend"
            value={formatTotal(totalActual)}
            subtitle={budgetUsedPct !== null ? `${budgetUsedPct}% of budget` : "No budget yet"}
            trend={
              spendTrendPct !== null
                ? {
                    direction: Number(spendTrendPct) >= 0 ? "up" : "down",
                    value: `${spendTrendPct}% vs last month`,
                  }
                : undefined
            }
            icon={<TrendingUp className="h-4 w-4" />}
          />

          <KPICard
            title="Pending Approvals"
            value={pendingApprovals ?? "—"}
            subtitle="Awaiting review"
            variant={pendingApprovals !== undefined && pendingApprovals > 5 ? "warning" : "default"}
            icon={<Clock className="h-4 w-4" />}
          />
        </div>
      </section>

      {/* ── Row 2: Portfolio Health + Burn Rate ─────────────────────── */}
      <section>
        <SectionHeading>Portfolio &amp; Financial Overview</SectionHeading>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <BudgetHealthPie
            onTrack={portfolio?.health_counts.on_track ?? 0}
            atRisk={portfolio?.health_counts.at_risk ?? 0}
            overBudget={portfolio?.health_counts.over_budget ?? 0}
          />
          <BurnRateChart data={burnRateData} />
        </div>
      </section>

      {/* ── Row 3: Approval Pipeline + Team Utilization ─────────────── */}
      <section>
        <SectionHeading>Operations</SectionHeading>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ApprovalAgeBars
            under24h={approvals?.by_age.under_24h ?? 0}
            oneToThreeDays={approvals?.by_age.days_1_to_3 ?? 0}
            threeToSevenDays={approvals?.by_age.days_3_to_7 ?? 0}
            overSevenDays={approvals?.by_age.over_7_days ?? 0}
          />
          <UtilizationGauge
            percentage={Number(team?.utilization_pct ?? 0)}
            label={`${team?.team_member_label ?? entityLabels.teamMemberPlural} Utilization`}
            assigned={team?.assigned ?? 0}
            total={team?.total_members ?? 0}
          />
        </div>
      </section>

      {/* ── Row 4: Compliance Status ────────────────────────────────── */}
      <section>
        <SectionHeading>Compliance Status</SectionHeading>

        {/* Count badges */}
        <div className="mb-4 flex flex-wrap gap-4">
          <div
            className="flex items-center gap-2 rounded-lg px-4 py-2"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <AlertTriangle className="h-4 w-4 text-amber-500" />
            <span className="text-sm font-heading font-medium" style={{ color: "var(--thittam-foreground, #0f172a)" }}>
              Overdue Approvals
            </span>
            <span className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-amber-100 px-1.5 text-xs font-mono font-bold text-amber-700">
              {compliance?.overdue_approvals ?? 0}
            </span>
          </div>

          <div
            className="flex items-center gap-2 rounded-lg px-4 py-2"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <ShieldAlert className="h-4 w-4 text-red-500" />
            <span className="text-sm font-heading font-medium" style={{ color: "var(--thittam-foreground, #0f172a)" }}>
              Policy Violations
            </span>
            <span className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-red-100 px-1.5 text-xs font-mono font-bold text-red-700">
              {compliance?.policy_violations ?? 0}
            </span>
          </div>

          <div
            className="flex items-center gap-2 rounded-lg px-4 py-2"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <Flag className="h-4 w-4 text-gray-400" />
            <span className="text-sm font-heading font-medium" style={{ color: "var(--thittam-foreground, #0f172a)" }}>
              Audit Flags
            </span>
            <span className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-gray-100 px-1.5 text-xs font-mono font-bold text-gray-600">
              {compliance?.audit_flags ?? 0}
            </span>
          </div>
        </div>

        {/* Violations table */}
        <ComplianceTable items={complianceItems} isLoading={complianceQuery.isLoading} />
      </section>

      {/* ── Row 5: Recent Activity ──────────────────────────────────── */}
      <section>
        <SectionHeading>Recent Activity</SectionHeading>
        <div
          className="rounded-xl p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          {/* No activity-feed endpoint exists yet; the timeline shows its empty state. */}
          <ActivityTimeline items={[]} />
        </div>
      </section>

      {/* ── Quick Actions ───────────────────────────────────────────── */}
      <section>
        <SectionHeading>Quick Actions</SectionHeading>
        <div className="flex flex-wrap items-center gap-3">
          <Link
            href="/productions/new"
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold font-heading transition-opacity hover:opacity-90"
            style={{
              backgroundColor: "var(--thittam-primary, #3b82f6)",
              color: "var(--thittam-primary-foreground, #fff)",
            }}
          >
            <Plus className="h-4 w-4" />
            Create New {entityLabels.project}
          </Link>

          <Link
            href="/productions"
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold font-heading transition-colors hover:opacity-80"
            style={{
              border: "1px solid var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
            }}
          >
            View All {entityLabels.projectPlural}
            <ArrowRight className="h-4 w-4" />
          </Link>

          <Link
            href="/approvals"
            className="relative inline-flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold font-heading transition-colors hover:opacity-80"
            style={{
              border: "1px solid var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
            }}
          >
            <CheckCircle2 className="h-4 w-4" />
            Pending Approvals
            <span
              className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full px-1.5 text-xs font-mono font-bold"
              style={{
                backgroundColor: "var(--thittam-primary, #3b82f6)",
                color: "var(--thittam-primary-foreground, #fff)",
              }}
            >
              {pendingApprovals ?? "—"}
            </span>
          </Link>
        </div>
      </section>
    </div>
  );
}
