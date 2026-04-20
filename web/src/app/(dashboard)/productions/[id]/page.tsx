"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Archive } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { StatusBadge } from "@/components/ui/status-badge";
import {
  useProduction,
  useArchiveProduction,
} from "@/lib/hooks/use-productions";

const TABS = ["Overview", "Phases", "Crew", "Budget"] as const;
type Tab = (typeof TABS)[number];

// Phases, crew, and budget-summary aggregation are not wired onto the live
// API yet — those three tabs render "coming in v0.20.1" placeholders for
// now. The overview tab shows the real production record; archive works via
// useArchiveProduction.
function ComingSoonTab({ message }: { message: string }) {
  return (
    <div
      className="rounded-xl p-6"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        border: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      <div
        className="rounded-lg border-dashed p-8 text-center"
        style={{ border: "2px dashed var(--thittam-border, #e2e8f0)" }}
      >
        <p
          className="text-sm font-body"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {message}
        </p>
      </div>
    </div>
  );
}

export default function ProductionDetailPage() {
  const { entityLabels } = useTheme();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const [activeTab, setActiveTab] = useState<Tab>("Overview");

  const query = useProduction(params.id);
  const archive = useArchiveProduction();
  const production = query.data;

  if (query.isLoading) {
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

  if (query.error || !production) {
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
                Budget aggregation coming in v0.20.1. See{" "}
                <Link
                  href="/budgets"
                  className="font-heading font-medium"
                  style={{ color: "var(--thittam-primary, #3b82f6)" }}
                >
                  Budgets
                </Link>{" "}
                for the live data.
              </p>
            </div>
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
        <ComingSoonTab
          message={`${entityLabels.phase} timeline wiring is coming in v0.20.1 — the API endpoints are live on :9080 but the UI mapping (API phase.phase_type → displayed label) is still being finalised.`}
        />
      )}

      {activeTab === "Crew" && (
        <ComingSoonTab
          message={`${entityLabels.teamMember} roster wiring is coming in v0.20.1 — the API is live, UI mapping pending.`}
        />
      )}

      {activeTab === "Budget" && (
        <ComingSoonTab
          message="Detailed budget breakdown per production is coming in v0.20.1. See the Budgets page for the live budget list."
        />
      )}
    </div>
  );
}
