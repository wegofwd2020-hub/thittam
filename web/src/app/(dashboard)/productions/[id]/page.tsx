"use client";

import { useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Archive } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { StatusBadge } from "@/components/ui/status-badge";
import {
  PhaseTimeline,
  type Phase,
} from "@/components/productions/phase-timeline";
import {
  CrewTable,
  type CrewMember,
} from "@/components/productions/crew-table";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------
interface ProductionDetail {
  id: string;
  title: string;
  status: string;
  description: string;
  genre: string;
  startDate: string;
  endDate?: string;
  budgetTotal: number;
  budgetSpent: number;
  budgetHealth: "on_track" | "at_risk" | "over_budget";
}

const mockProductions: Record<string, ProductionDetail> = {
  "1": {
    id: "1",
    title: "The Last Horizon",
    status: "production",
    description:
      "A sci-fi thriller following a crew of astronauts who discover an anomaly at the edge of the solar system that challenges everything they know about reality.",
    genre: "Sci-Fi",
    startDate: "2026-01-15",
    endDate: "2026-08-30",
    budgetTotal: 2500000,
    budgetSpent: 1475000,
    budgetHealth: "on_track",
  },
  "2": {
    id: "2",
    title: "Midnight Express Reboot",
    status: "post_production",
    description:
      "A modern reimagining of the classic thriller, set in contemporary Istanbul with a focus on digital-age surveillance and personal freedom.",
    genre: "Thriller",
    startDate: "2025-06-01",
    endDate: "2026-03-15",
    budgetTotal: 1800000,
    budgetSpent: 1720000,
    budgetHealth: "at_risk",
  },
  "3": {
    id: "3",
    title: "Project Starfall",
    status: "development",
    description:
      "An animated feature exploring a young girl's journey through a world where fallen stars grant wishes — but at a hidden cost.",
    genre: "Animation",
    startDate: "2026-03-01",
    budgetTotal: 3200000,
    budgetSpent: 120000,
    budgetHealth: "on_track",
  },
};

const mockPhases: Phase[] = [
  {
    id: "dev",
    type: "development",
    label: "Development",
    status: "completed",
    billable: false,
    startDate: "2026-01-15",
    endDate: "2026-02-28",
  },
  {
    id: "pre",
    type: "pre_production",
    label: "Pre-Production",
    status: "completed",
    billable: true,
    startDate: "2026-03-01",
    endDate: "2026-03-31",
  },
  {
    id: "prod",
    type: "production",
    label: "Production",
    status: "current",
    billable: true,
    startDate: "2026-04-01",
  },
  {
    id: "post",
    type: "post_production",
    label: "Post-Production",
    status: "upcoming",
    billable: true,
  },
  {
    id: "release",
    type: "released",
    label: "Release",
    status: "upcoming",
    billable: false,
  },
];

const mockAllowedTransitions = [
  { from: "prod", to: "post" },
];

const mockCrew: CrewMember[] = [
  {
    id: "c1",
    name: "Sarah Chen",
    role: "Director",
    department: "Direction",
    dayRate: 5000,
    startDate: "2026-01-15",
  },
  {
    id: "c2",
    name: "Marcus Johnson",
    role: "Director of Photography",
    department: "Camera",
    dayRate: 3500,
    startDate: "2026-03-01",
  },
  {
    id: "c3",
    name: "Aisha Patel",
    role: "Production Designer",
    department: "Art",
    dayRate: 2800,
    startDate: "2026-02-15",
  },
  {
    id: "c4",
    name: "James Liu",
    role: "Sound Supervisor",
    department: "Sound",
    dayRate: 2200,
    startDate: "2026-04-01",
  },
  {
    id: "c5",
    name: "Elena Rodriguez",
    role: "Line Producer",
    department: "Production",
    dayRate: 3000,
    startDate: "2026-01-15",
  },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amount);
}

// ---------------------------------------------------------------------------
// Tabs
// ---------------------------------------------------------------------------
const TABS = ["Overview", "Phases", "Crew", "Budget"] as const;
type Tab = (typeof TABS)[number];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function ProductionDetailPage() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const [activeTab, setActiveTab] = useState<Tab>("Overview");
  const [crew, setCrew] = useState<CrewMember[]>(mockCrew);
  const [phases, setPhases] = useState<Phase[]>(mockPhases);

  const production = mockProductions[params.id];

  if (!production) {
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
          The {entityLabels.project.toLowerCase()} you are looking for does not
          exist or has been removed.
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

  const budgetPct =
    production.budgetTotal > 0
      ? Math.round((production.budgetSpent / production.budgetTotal) * 100)
      : 0;

  function handleArchive() {
    // Placeholder — would call API
    router.push("/productions");
  }

  function handlePhaseTransition(fromId: string, toId: string) {
    setPhases((prev) =>
      prev.map((p) => {
        if (p.id === fromId) return { ...p, status: "completed" as const };
        if (p.id === toId) return { ...p, status: "current" as const };
        return p;
      })
    );
  }

  function handleAddCrew(member: Omit<CrewMember, "id">) {
    setCrew((prev) => [
      ...prev,
      { ...member, id: `c${Date.now()}` },
    ]);
  }

  function handleRemoveCrew(memberId: string) {
    setCrew((prev) => prev.filter((m) => m.id !== memberId));
  }

  return (
    <div className="mx-auto max-w-5xl">
      {/* Back link */}
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
          className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-heading font-medium transition-colors hover:bg-gray-100"
          style={{
            color: "var(--thittam-muted-foreground, #64748b)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <Archive className="h-4 w-4" />
          Archive
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

      {/* Tab content */}
      {activeTab === "Overview" && (
        <div className="grid gap-6 lg:grid-cols-3">
          {/* Details */}
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
                <dt
                  className="font-heading text-xs font-medium text-gray-500"
                >
                  Genre
                </dt>
                <dd
                  className="mt-0.5 font-body"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {production.genre}
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
                  {production.startDate}
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
                  {production.endDate ?? "TBD"}
                </dd>
              </div>
            </dl>
          </div>

          {/* Budget summary card */}
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
            <div className="flex flex-col gap-3">
              <div>
                <span className="font-heading text-xs text-gray-500">
                  Total Budget
                </span>
                <p
                  className="font-mono text-lg font-semibold"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {formatCurrency(production.budgetTotal)}
                </p>
              </div>
              <div>
                <span className="font-heading text-xs text-gray-500">
                  Spent
                </span>
                <p
                  className="font-mono text-lg font-semibold"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {formatCurrency(production.budgetSpent)}
                </p>
              </div>

              {/* Progress bar */}
              <div>
                <div className="mb-1 flex items-center justify-between">
                  <span className="font-heading text-xs text-gray-500">
                    Utilization
                  </span>
                  <span className="font-mono text-xs font-medium">
                    {budgetPct}%
                  </span>
                </div>
                <div
                  className="h-2 w-full overflow-hidden rounded-full"
                  style={{
                    backgroundColor: "var(--thittam-muted, #f1f5f9)",
                  }}
                >
                  <div
                    className="h-full rounded-full transition-all"
                    style={{
                      width: `${Math.min(budgetPct, 100)}%`,
                      backgroundColor:
                        production.budgetHealth === "on_track"
                          ? "var(--thittam-status-on-track, #16A34A)"
                          : production.budgetHealth === "at_risk"
                            ? "var(--thittam-status-at-risk, #F59E0B)"
                            : "var(--thittam-status-over-budget, #DC2626)",
                    }}
                  />
                </div>
              </div>

              <StatusBadge status={production.budgetHealth} variant="health" />
            </div>
          </div>

          {/* Recent activity placeholder */}
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
          <PhaseTimeline
            phases={phases}
            allowedTransitions={mockAllowedTransitions}
            onTransition={handlePhaseTransition}
          />
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
          <CrewTable
            members={crew}
            onAdd={handleAddCrew}
            onRemove={handleRemoveCrew}
          />
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
          <h2
            className="mb-4 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Budget Details
          </h2>
          <div
            className="rounded-lg border-dashed p-8 text-center"
            style={{ border: "2px dashed var(--thittam-border, #e2e8f0)" }}
          >
            <p
              className="text-sm font-body"
              style={{
                color: "var(--thittam-muted-foreground, #64748b)",
              }}
            >
              Detailed budget breakdown will be available when the
              budget-planning service is integrated.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
