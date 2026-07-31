"use client";

import { useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Archive } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import {
  PhaseTimeline,
  type Phase as UIPhase,
} from "@/components/productions/phase-timeline";
import {
  CrewTable,
  type CrewMember as UICrewMember,
} from "@/components/productions/crew-table";
import {
  useProduction,
  useArchiveProduction,
  usePhases,
  usePhaseTypes,
  useUpdatePhaseStatus,
  useCrewMembers,
  useAddCrewMember,
  useRemoveCrewMember,
} from "@/lib/hooks/use-productions";
import {
  useProductionBudgetSummary,
  type ProductionBudgetSummary,
} from "@/lib/hooks/use-production-budget-summary";
import type {
  Phase as ApiPhase,
  PhaseType,
  CrewMember as ApiCrewMember,
} from "@/lib/api/productions";

const HEALTH_BAR_COLOR: Record<
  ProductionBudgetSummary["health"],
  string
> = {
  on_track: "var(--thittam-status-on-track, #16A34A)",
  at_risk: "var(--thittam-status-at-risk, #F59E0B)",
  over_budget: "var(--thittam-status-over-budget, #DC2626)",
};

const TABS = ["Overview", "Phases", "Crew", "Budget"] as const;
type Tab = (typeof TABS)[number];

// Maps the API's three-state phase.status onto the component's
// upcoming/current/completed vocabulary. Anything unknown (shouldn't
// happen — the DB CHECK constraint covers these three) falls back to
// "upcoming" so the UI renders something benign.
function mapPhaseStatus(api: string): UIPhase["status"] {
  switch (api) {
    case "completed":
      return "completed";
    case "active":
      return "current";
    case "pending":
    default:
      return "upcoming";
  }
}

// Joins API Phase rows against the vertical's PhaseType definitions
// (which carry the human-readable label, billable flag, and allowed
// transitions). Sort order matches PhaseType.order so the timeline
// renders in vertical-defined sequence rather than creation order.
function joinPhases(phases: ApiPhase[], types: PhaseType[]): UIPhase[] {
  const byId = new Map(types.map((t) => [t.id, t]));
  const orderIndex = (p: ApiPhase) => byId.get(p.phase_type)?.order ?? 999;
  return [...phases]
    .sort((a, b) => orderIndex(a) - orderIndex(b))
    .map((p) => {
      const t = byId.get(p.phase_type);
      return {
        id: p.id,
        type: p.phase_type,
        label: t?.label ?? p.phase_type,
        status: mapPhaseStatus(p.status),
        billable: t?.is_billable ?? false,
        startDate: p.start_date || undefined,
        endDate: p.end_date || undefined,
      };
    });
}

// The PhaseTimeline component takes allowed transitions as {from, to}
// pairs referencing *phase IDs* (not phase-type IDs). Walk the joined
// phase list: from the current phase, the allowed destinations come
// from PhaseType.allowed_transitions, resolved back to the matching
// phase rows on this production.
function computeAllowedTransitions(
  uiPhases: UIPhase[],
  types: PhaseType[],
): Array<{ from: string; to: string }> {
  const current = uiPhases.find((p) => p.status === "current");
  if (!current) return [];
  const currentType = types.find((t) => t.id === current.type);
  if (!currentType) return [];
  const targets = new Set(currentType.allowed_transitions);
  return uiPhases
    .filter((p) => targets.has(p.type) && p.status === "upcoming")
    .map((p) => ({ from: current.id, to: p.id }));
}

// API day_rate is a Decimal-in-a-string (Rule #1). The CrewTable
// component expects number — lossy at extreme precision but correct
// within Number.MAX_SAFE_INTEGER / 100 for day-rate values. If we ever
// need real rounding discipline here, change the component to string.
function toUICrew(m: ApiCrewMember): UICrewMember {
  return {
    id: m.id,
    name: m.name,
    role: m.role,
    department: m.department,
    dayRate: Number.parseFloat(m.day_rate) || 0,
    startDate: m.start_date ?? "",
    endDate: m.end_date ?? undefined,
  };
}

interface BudgetSummaryProps {
  summary: ProductionBudgetSummary | null;
  hasBudget: boolean;
  isLoading: boolean;
  error: Error | null;
}

function BudgetSummaryCard({
  summary,
  hasBudget,
  isLoading,
  error,
}: BudgetSummaryProps) {
  if (isLoading) {
    return (
      <p
        className="font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Loading budget…
      </p>
    );
  }
  if (error) {
    return (
      <p
        className="font-body text-sm"
        style={{ color: "var(--thittam-destructive, #DC2626)" }}
      >
        Failed to load: {error.message}
      </p>
    );
  }
  if (!hasBudget || !summary) {
    return (
      <p
        className="font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        No budget yet.{" "}
        <Link
          href="/budgets/new"
          className="font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          Create one
        </Link>
        .
      </p>
    );
  }

  const pct = Math.round(summary.utilizationPct);
  return (
    <div className="flex flex-col gap-3">
      <div>
        <span className="font-heading text-xs text-gray-500">Total Budget</span>
        <p
          className="font-mono text-lg font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          <AmountDisplay
            amount={summary.totalBudgeted}
            currency={summary.currency}
            size="lg"
          />
        </p>
      </div>
      <div>
        <span className="font-heading text-xs text-gray-500">Spent</span>
        <p
          className="font-mono text-lg font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          <AmountDisplay
            amount={summary.totalActual}
            currency={summary.currency}
            size="lg"
          />
        </p>
      </div>
      <div>
        <div className="mb-1 flex items-center justify-between">
          <span className="font-heading text-xs text-gray-500">Utilization</span>
          <span className="font-mono text-xs font-medium">{pct}%</span>
        </div>
        <div
          className="h-2 w-full overflow-hidden rounded-full"
          style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
        >
          <div
            className="h-full rounded-full transition-all"
            style={{
              width: `${Math.min(pct, 100)}%`,
              backgroundColor: HEALTH_BAR_COLOR[summary.health],
            }}
          />
        </div>
      </div>
      <div className="flex items-center justify-between">
        <StatusBadge status={summary.health} variant="health" />
        <Link
          href={`/budgets/${summary.budget.id}`}
          className="text-xs font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          View {summary.budget.label} →
        </Link>
      </div>
    </div>
  );
}

function BudgetTabPanel({
  summary,
  hasBudget,
  isLoading,
  error,
}: BudgetSummaryProps) {
  if (isLoading) {
    return (
      <p
        className="font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Loading budget details…
      </p>
    );
  }
  if (error) {
    return (
      <p
        className="font-body text-sm"
        style={{ color: "var(--thittam-destructive, #DC2626)" }}
      >
        Failed to load: {error.message}
      </p>
    );
  }
  if (!hasBudget || !summary) {
    return (
      <div
        className="rounded-lg border-dashed p-8 text-center"
        style={{ border: "2px dashed var(--thittam-border, #e2e8f0)" }}
      >
        <p
          className="text-sm font-body"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          No budget has been created for this production yet.
        </p>
        <Link
          href="/budgets/new"
          className="mt-4 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white"
          style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
        >
          Create Budget
        </Link>
      </div>
    );
  }

  const pct = Math.round(summary.utilizationPct);
  return (
    <div>
      {/* Header row: label + version status + link to the full budget page */}
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h2
            className="font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {summary.budget.label}
          </h2>
          <StatusBadge status={summary.budget.status} variant="budget" />
        </div>
        <Link
          href={`/budgets/${summary.budget.id}`}
          className="text-xs font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          View full budget →
        </Link>
      </div>

      {/* KPI grid: budgeted / actual / committed / remaining */}
      <div className="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <KpiCell
          label="Total Budget"
          amount={summary.totalBudgeted}
          currency={summary.currency}
        />
        <KpiCell
          label="Spent"
          amount={summary.totalActual}
          currency={summary.currency}
        />
        <KpiCell
          label="Committed"
          amount={summary.totalCommitted}
          currency={summary.currency}
        />
        <KpiCell
          label="Remaining"
          amount={summary.remaining}
          currency={summary.currency}
        />
      </div>

      {/* Utilization bar */}
      <div className="mb-4">
        <div className="mb-1 flex items-center justify-between">
          <span className="font-heading text-xs text-gray-500">Utilization</span>
          <span className="font-mono text-xs font-medium">{pct}%</span>
        </div>
        <div
          className="h-3 w-full overflow-hidden rounded-full"
          style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
        >
          <div
            className="h-full rounded-full transition-all"
            style={{
              width: `${Math.min(pct, 100)}%`,
              backgroundColor: HEALTH_BAR_COLOR[summary.health],
            }}
          />
        </div>
      </div>

      <StatusBadge status={summary.health} variant="health" />
    </div>
  );
}

function KpiCell({
  label,
  amount,
  currency,
}: {
  label: string;
  amount: number;
  currency: string;
}) {
  return (
    <div
      className="rounded-lg p-3"
      style={{ backgroundColor: "var(--thittam-muted, #f8fafc)" }}
    >
      <span className="font-heading text-xs text-gray-500">{label}</span>
      <p
        className="mt-1 font-mono text-base font-semibold"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        <AmountDisplay amount={amount} currency={currency} size="md" />
      </p>
    </div>
  );
}

export function ProductionDetailView() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const [activeTab, setActiveTab] = useState<Tab>("Overview");

  const productionQuery = useProduction(params.id);
  const phasesQuery = usePhases(params.id);
  const phaseTypesQuery = usePhaseTypes();
  const crewQuery = useCrewMembers(params.id);
  const budgetSummary = useProductionBudgetSummary(params.id);

  const archive = useArchiveProduction();
  const updatePhase = useUpdatePhaseStatus();
  const addCrew = useAddCrewMember(params.id);
  const removeCrew = useRemoveCrewMember();

  const production = productionQuery.data;

  const uiPhases = useMemo(
    () => joinPhases(phasesQuery.data ?? [], phaseTypesQuery.data ?? []),
    [phasesQuery.data, phaseTypesQuery.data],
  );

  const allowedTransitions = useMemo(
    () => computeAllowedTransitions(uiPhases, phaseTypesQuery.data ?? []),
    [uiPhases, phaseTypesQuery.data],
  );

  const uiCrew = useMemo(
    () => (crewQuery.data ?? []).map(toUICrew),
    [crewQuery.data],
  );

  if (productionQuery.isLoading) {
    return (
      <div className="mx-auto max-w-5xl py-12 text-center">
        <p
          className="font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Loading {entityLabels.project.toLowerCase()}…
        </p>
      </div>
    );
  }

  if (productionQuery.error || !production) {
    return (
      <div className="mx-auto max-w-4xl py-12 text-center">
        <h1
          className="font-heading text-xl font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {entityLabels.project} not found
        </h1>
        <p
          className="mt-2 font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          The {entityLabels.project.toLowerCase()} you are looking for does
          not exist or has been removed.
        </p>
        <Link
          href="/productions"
          className="mt-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          <ArrowLeft className="h-4 w-4" />
          Back to {entityLabels.projectPlural}
        </Link>
      </div>
    );
  }

  function handleArchive() {
    if (!production) return;
    archive.mutate(production.id, {
      onSuccess: () => router.push("/productions"),
    });
  }

  // PhaseTimeline passes the *target* phase id on transition. The current
  // phase is always our mutation's source; we look up its phase_type (the
  // new status to set) from the UI list the timeline already has.
  function handlePhaseTransition(_fromId: string, toId: string) {
    const target = uiPhases.find((p) => p.id === toId);
    if (!target) return;
    updatePhase.mutate({ phaseId: toId, newPhaseType: target.type });
  }

  return (
    <div className="mx-auto max-w-5xl">
      <Link
        href="/productions"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        {entityLabels.projectPlural}
      </Link>

      {/* Header */}
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <h1
            className="font-heading text-2xl font-semibold tracking-tight"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {production.title}
          </h1>
          <StatusBadge status={production.status} />
        </div>
        <button
          onClick={handleArchive}
          disabled={archive.isPending}
          className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium transition-colors hover:bg-gray-100 disabled:opacity-50"
          style={{
            color: "var(--thittam-muted-foreground, #64748b)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <Archive className="h-4 w-4" />
          {archive.isPending ? "Archiving…" : "Archive"}
        </button>
      </div>

      {/* Tabs */}
      <div
        className="mb-6 flex gap-0 overflow-x-auto"
        style={{ borderBottom: "1px solid var(--thittam-border, #e2e8f0)" }}
      >
        {TABS.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className="relative px-4 py-2.5 text-sm font-heading font-medium transition-colors"
            style={{
              color:
                activeTab === tab
                  ? "var(--thittam-primary, #3b82f6)"
                  : "var(--thittam-muted-foreground, #64748b)",
            }}
          >
            {tab}
            {activeTab === tab && (
              <span
                className="absolute bottom-0 left-0 right-0 h-0.5"
                style={{
                  backgroundColor: "var(--thittam-primary, #3b82f6)",
                }}
              />
            )}
          </button>
        ))}
      </div>

      {activeTab === "Overview" && (
        <div className="grid gap-6 lg:grid-cols-3">
          <div
            className="rounded-xl p-5 lg:col-span-2"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <h2
              className="mb-3 font-heading text-sm font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Details
            </h2>
            <p
              className="mb-4 font-body text-sm leading-relaxed"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {production.description}
            </p>
            <dl className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <dt className="font-heading text-xs font-medium text-gray-500">
                  Genre
                </dt>
                <dd
                  className="mt-0.5 font-body"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {production.genre || "—"}
                </dd>
              </div>
              <div>
                <dt className="font-heading text-xs font-medium text-gray-500">
                  Status
                </dt>
                <dd className="mt-0.5">
                  <StatusBadge status={production.status} />
                </dd>
              </div>
              <div>
                <dt className="font-heading text-xs font-medium text-gray-500">
                  Start Date
                </dt>
                <dd
                  className="mt-0.5 font-mono"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {production.start_date ?? "TBD"}
                </dd>
              </div>
              <div>
                <dt className="font-heading text-xs font-medium text-gray-500">
                  End Date
                </dt>
                <dd
                  className="mt-0.5 font-mono"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {production.end_date ?? "TBD"}
                </dd>
              </div>
            </dl>
          </div>

          <div
            className="rounded-xl p-5"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <h2
              className="mb-3 font-heading text-sm font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Budget Summary
            </h2>
            <BudgetSummaryCard
              summary={budgetSummary.data}
              hasBudget={budgetSummary.hasBudget}
              isLoading={budgetSummary.isLoading}
              error={budgetSummary.error}
            />
          </div>

          <div
            className="rounded-xl p-5 lg:col-span-3"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <h2
              className="mb-3 font-heading text-sm font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Recent Activity
            </h2>
            <div
              className="rounded-lg border-dashed p-6 text-center"
              style={{ border: "2px dashed var(--thittam-border, #e2e8f0)" }}
            >
              <p
                className="text-sm font-body"
                style={{
                  color: "var(--thittam-muted-foreground, #64748b)",
                }}
              >
                Activity feed will be available in a future release.
              </p>
            </div>
          </div>
        </div>
      )}

      {activeTab === "Phases" && (
        <div
          className="rounded-xl p-6"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <h2
            className="mb-4 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {entityLabels.phase} Timeline
          </h2>
          {phasesQuery.isLoading || phaseTypesQuery.isLoading ? (
            <p
              className="font-body text-sm"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Loading…
            </p>
          ) : phasesQuery.error ? (
            <p
              className="font-body text-sm"
              style={{ color: "var(--thittam-destructive, #DC2626)" }}
            >
              Failed to load phases: {phasesQuery.error.message}
            </p>
          ) : uiPhases.length === 0 ? (
            <p
              className="font-body text-sm"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              No phases yet for this {entityLabels.project.toLowerCase()}.
            </p>
          ) : (
            <PhaseTimeline
              phases={uiPhases}
              allowedTransitions={allowedTransitions}
              onTransition={handlePhaseTransition}
            />
          )}
        </div>
      )}

      {activeTab === "Crew" && (
        <div
          className="rounded-xl p-6"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          {crewQuery.isLoading ? (
            <p
              className="font-body text-sm"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Loading {entityLabels.teamMemberPlural.toLowerCase()}…
            </p>
          ) : crewQuery.error ? (
            <p
              className="font-body text-sm"
              style={{ color: "var(--thittam-destructive, #DC2626)" }}
            >
              Failed to load: {crewQuery.error.message}
            </p>
          ) : (
            <CrewTable
              members={uiCrew}
              onAdd={(m) =>
                addCrew.mutate({
                  name: m.name,
                  role: m.role,
                  department: m.department,
                  day_rate: String(m.dayRate),
                })
              }
              onRemove={(id) => removeCrew.mutate(id)}
            />
          )}
        </div>
      )}

      {activeTab === "Budget" && (
        <div
          className="rounded-xl p-6"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <BudgetTabPanel
            summary={budgetSummary.data}
            hasBudget={budgetSummary.hasBudget}
            isLoading={budgetSummary.isLoading}
            error={budgetSummary.error}
          />
        </div>
      )}
    </div>
  );
}
