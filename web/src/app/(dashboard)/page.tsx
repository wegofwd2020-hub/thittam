"use client";

import { useAuth } from "@/lib/auth/context";
import { useTheme } from "@/lib/themes/provider";
import { KPICard } from "@/components/dashboard/kpi-card";
import { BudgetHealthPie } from "@/components/dashboard/budget-health-pie";
import { BurnRateChart } from "@/components/dashboard/burn-rate-chart";
import { ApprovalAgeBars } from "@/components/dashboard/approval-age-bars";
import { UtilizationGauge } from "@/components/dashboard/utilization-gauge";
import { ComplianceTable, type ComplianceItem } from "@/components/dashboard/compliance-table";
import { ActivityTimeline, type ActivityItem } from "@/components/dashboard/activity-timeline";
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

// ---------------------------------------------------------------------------
// Mock data — XYZ_CBA demo company
// ---------------------------------------------------------------------------

const MOCK_PRODUCTIONS = [
  {
    id: "prod-001",
    name: "Naan Mudhalvan",
    status: "production",
    health: "on_track",
    budget: 10_00_00_000,
    actual: 7_20_00_000,
    teamCount: 15,
  },
  {
    id: "prod-002",
    name: "Ponniyin Selvan III",
    status: "pre_production",
    health: "on_track",
    budget: 9_50_00_000,
    actual: 4_80_00_000,
    teamCount: 12,
  },
  {
    id: "prod-003",
    name: "Vikram 2",
    status: "post_production",
    health: "on_track",
    budget: 5_00_00_000,
    actual: 3_50_00_000,
    teamCount: 5,
  },
  {
    id: "prod-004",
    name: "Master Class",
    status: "production",
    health: "on_track",
    budget: 6_00_00_000,
    actual: 3_00_00_000,
    teamCount: 10,
  },
  {
    id: "prod-005",
    name: "Jailer Returns",
    status: "pre_production",
    health: "on_track",
    budget: 8_00_00_000,
    actual: 1_50_00_000,
    teamCount: 8,
  },
  {
    id: "prod-006",
    name: "Beast Mode",
    status: "production",
    health: "at_risk",
    budget: 4_50_00_000,
    actual: 3_80_00_000,
    teamCount: 7,
  },
  {
    id: "prod-007",
    name: "Kaithi 2",
    status: "production",
    health: "at_risk",
    budget: 7_00_00_000,
    actual: 6_20_00_000,
    teamCount: 9,
  },
  {
    id: "prod-008",
    name: "Don 3",
    status: "post_production",
    health: "over_budget",
    budget: 3_00_00_000,
    actual: 3_40_00_000,
    teamCount: 4,
  },
];

const TOTAL_BUDGET = 53_00_00_000;
const TOTAL_ACTUAL = 33_40_00_000;
const LAST_MONTH_ACTUAL = 28_50_00_000;

const BURN_RATE_DATA = [
  { month: "Nov 2025", actual: 3_80_00_000, budget: 4_00_00_000 },
  { month: "Dec 2025", actual: 4_50_00_000, budget: 4_00_00_000 },
  { month: "Jan 2026", actual: 5_20_00_000, budget: 4_50_00_000 },
  { month: "Feb 2026", actual: 5_80_00_000, budget: 5_00_00_000 },
  { month: "Mar 2026", actual: 6_10_00_000, budget: 5_50_00_000 },
  { month: "Apr 2026", actual: 4_00_00_000, budget: 5_50_00_000 },
];

const MOCK_COMPLIANCE: ComplianceItem[] = [
  {
    id: "c-1",
    type: "Overdue Approval",
    description: "PO #4521 pending for 12 days without action",
    severity: "critical",
    detectedAt: "2026-04-02",
    projectTitle: "Kaithi 2",
  },
  {
    id: "c-2",
    type: "Policy Violation",
    description: "Expense submitted without receipt attachment",
    severity: "warning",
    detectedAt: "2026-04-01",
    projectTitle: "Beast Mode",
  },
  {
    id: "c-3",
    type: "Budget Overrun",
    description: "Don 3 post-production spend exceeds approved budget by 13%",
    severity: "critical",
    detectedAt: "2026-03-30",
    projectTitle: "Don 3",
  },
  {
    id: "c-4",
    type: "Audit Flag",
    description: "Duplicate vendor payment detected for catering PO",
    severity: "warning",
    detectedAt: "2026-03-28",
    projectTitle: "Naan Mudhalvan",
  },
];

// Timestamps relative to "now" — using fixed dates for deterministic display
const now = new Date();
const hours = (h: number) => new Date(now.getTime() - h * 3_600_000).toISOString();

const MOCK_ACTIVITY: ActivityItem[] = [
  {
    id: "a-1",
    actor: "Priya Sharma",
    action: "approved expense",
    target: "\u20B91,50,000 — Camera Equipment Rental",
    timestamp: hours(2),
  },
  {
    id: "a-2",
    actor: "Arun Kumar",
    action: "submitted budget",
    target: "V2 — Ponniyin Selvan III",
    timestamp: hours(5),
  },
  {
    id: "a-3",
    actor: "Deepa Rajan",
    action: "created purchase order",
    target: "PO #4580 — Set Construction Phase 3",
    timestamp: hours(8),
  },
  {
    id: "a-4",
    actor: "Vikram Patel",
    action: "closed phase",
    target: "Post-Production — Don 3",
    timestamp: hours(18),
  },
  {
    id: "a-5",
    actor: "Meena Sundaram",
    action: "assigned crew to",
    target: "Jailer Returns — 8 crew members",
    timestamp: hours(26),
  },
];

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

  const activeCount = MOCK_PRODUCTIONS.length;
  const pendingApprovals = 8;
  const spendTrendPct = (
    ((TOTAL_ACTUAL - LAST_MONTH_ACTUAL) / LAST_MONTH_ACTUAL) *
    100
  ).toFixed(1);

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
      </div>

      {/* ── Row 1: KPI Cards ────────────────────────────────────────── */}
      <section>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <KPICard
            title={`Total ${entityLabels.projectPlural}`}
            value={activeCount}
            subtitle="Across all phases"
            icon={<FolderKanban className="h-4 w-4" />}
          />

          <KPICard
            title="Total Budget [INR]"
            value={`${(TOTAL_BUDGET / 1_00_00_000).toFixed(1)} Cr`}
            subtitle="Approved allocation"
            icon={<Wallet className="h-4 w-4" />}
          />

          <KPICard
            title="Total Actual Spend [INR]"
            value={`${(TOTAL_ACTUAL / 1_00_00_000).toFixed(1)} Cr`}
            subtitle={`${((TOTAL_ACTUAL / TOTAL_BUDGET) * 100).toFixed(1)}% of budget`}
            trend={{ direction: "up", value: `${spendTrendPct}% vs last month` }}
            icon={<TrendingUp className="h-4 w-4" />}
          />

          <KPICard
            title="Pending Approvals"
            value={pendingApprovals}
            subtitle="Awaiting review"
            variant={pendingApprovals > 5 ? "warning" : "default"}
            icon={<Clock className="h-4 w-4" />}
          />
        </div>
      </section>

      {/* ── Row 2: Portfolio Health + Burn Rate ─────────────────────── */}
      <section>
        <SectionHeading>Portfolio &amp; Financial Overview</SectionHeading>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <BudgetHealthPie onTrack={5} atRisk={2} overBudget={1} />
          <BurnRateChart data={BURN_RATE_DATA} />
        </div>
      </section>

      {/* ── Row 3: Approval Pipeline + Team Utilization ─────────────── */}
      <section>
        <SectionHeading>Operations</SectionHeading>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ApprovalAgeBars
            under24h={3}
            oneToThreeDays={2}
            threeToSevenDays={1}
            overSevenDays={2}
          />
          <UtilizationGauge
            percentage={80}
            label={`${entityLabels.teamMemberPlural} Utilization`}
            assigned={32}
            total={40}
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
              2
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
              1
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
              1
            </span>
          </div>
        </div>

        {/* Violations table */}
        <ComplianceTable items={MOCK_COMPLIANCE} />
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
          <ActivityTimeline items={MOCK_ACTIVITY} />
        </div>
      </section>

      {/* ── Quick Actions ───────────────────────────────────────────── */}
      <section>
        <SectionHeading>Quick Actions</SectionHeading>
        <div className="flex flex-wrap items-center gap-3">
          <Link
            href="/projects/new"
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
            href="/projects"
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
              {pendingApprovals}
            </span>
          </Link>
        </div>
      </section>
    </div>
  );
}
