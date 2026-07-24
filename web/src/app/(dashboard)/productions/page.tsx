"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { Plus, Search, LayoutGrid, List } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import {
  ProductionCard,
  type Production as CardProduction,
} from "@/components/productions/production-card";
import { StatusBadge } from "@/components/ui/status-badge";
import { useProductions } from "@/lib/hooks/use-productions";
import type { Production as ApiProduction } from "@/lib/api/productions";

// ---------------------------------------------------------------------------
// Filter chip definitions
// ---------------------------------------------------------------------------
const FILTER_OPTIONS = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "development", label: "Development" },
  { key: "pre_production", label: "Pre-Production" },
  { key: "production", label: "Production" },
  { key: "post_production", label: "Post-Production" },
  { key: "archived", label: "Archived" },
] as const;

type FilterKey = (typeof FILTER_OPTIONS)[number]["key"];

function matchesFilter(production: ApiProduction, filter: FilterKey): boolean {
  if (filter === "all") return true;
  if (filter === "active") return production.status !== "archived";
  return production.status === filter;
}

// Adapt the API shape onto the card's view-model. budgetHealth / currentPhase
// are derived data (phases + budgets) — intentionally omitted here; the card
// hides those indicators rather than guessing.
function toCardProduction(p: ApiProduction): CardProduction {
  return {
    id: p.id,
    title: p.title,
    status: p.status,
    description: p.description,
    genre: p.genre,
    startDate: p.start_date ?? undefined,
    endDate: p.end_date ?? undefined,
  };
}

function CardSkeleton() {
  return (
    <div
      className="animate-pulse rounded-xl p-5"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        border: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      <div className="mb-3 flex items-start justify-between">
        <div className="h-4 w-32 rounded bg-gray-200" />
        <div className="h-5 w-20 rounded-full bg-gray-200" />
      </div>
      <div className="mb-2 h-3 w-full rounded bg-gray-100" />
      <div className="mb-4 h-3 w-3/4 rounded bg-gray-100" />
      <div className="flex items-center justify-between">
        <div className="h-5 w-16 rounded bg-gray-100" />
        <div className="h-3 w-20 rounded bg-gray-100" />
      </div>
    </div>
  );
}

export default function ProductionsPage() {
  const { entityLabels } = useTheme();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<FilterKey>("all");
  const [viewMode, setViewMode] = useState<"grid" | "table">("grid");

  const query = useProductions();
  const productions = useMemo(
    () => query.data?.productions ?? [],
    [query.data],
  );

  const filtered = useMemo(() => {
    return productions.filter((p) => {
      if (!matchesFilter(p, filter)) return false;
      if (search) {
        const hay = `${p.title} ${p.description}`.toLowerCase();
        if (!hay.includes(search.toLowerCase())) return false;
      }
      return true;
    });
  }, [productions, search, filter]);

  const isLoading = query.isLoading;
  const errored = query.error;

  return (
    <div className="mx-auto max-w-6xl">
      {/* Page header */}
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1
            className="font-heading text-2xl font-semibold tracking-tight"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {entityLabels.projectPlural}
          </h1>
          <p
            className="mt-1 font-body text-sm"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Manage your {entityLabels.projectPlural.toLowerCase()} and track
            progress across {entityLabels.phasePlural.toLowerCase()}.
          </p>
        </div>
        <Link
          href="/productions/new"
          className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
          style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
        >
          <Plus className="h-4 w-4" />
          Create New
        </Link>
      </div>

      {/* Search + view toggle */}
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative flex-1 sm:max-w-sm">
          <Search
            className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          />
          <input
            type="text"
            placeholder={`Search ${entityLabels.projectPlural.toLowerCase()}...`}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-lg py-2 pl-9 pr-3 text-sm font-body"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
            }}
          />
        </div>

        <div
          className="flex items-center gap-1 rounded-lg p-0.5"
          style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
        >
          <button
            onClick={() => setViewMode("grid")}
            className={`rounded-md p-1.5 transition-colors ${
              viewMode === "grid" ? "bg-white shadow-sm" : ""
            }`}
            title="Grid view"
          >
            <LayoutGrid
              className="h-4 w-4"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            />
          </button>
          <button
            onClick={() => setViewMode("table")}
            className={`rounded-md p-1.5 transition-colors ${
              viewMode === "table" ? "bg-white shadow-sm" : ""
            }`}
            title="Table view"
          >
            <List
              className="h-4 w-4"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            />
          </button>
        </div>
      </div>

      {/* Filter chips */}
      <div className="mb-6 flex flex-wrap gap-2">
        {FILTER_OPTIONS.map((opt) => (
          <button
            key={opt.key}
            onClick={() => setFilter(opt.key)}
            className={`rounded-full px-3 py-1 text-xs font-heading font-medium transition-colors ${
              filter === opt.key ? "text-white" : ""
            }`}
            style={
              filter === opt.key
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

      {/* Content */}
      {isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      ) : errored ? (
        <div
          className="rounded-xl p-8 text-center"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <p
            className="font-heading text-sm font-medium"
            style={{ color: "var(--thittam-destructive, #DC2626)" }}
          >
            Failed to load {entityLabels.projectPlural.toLowerCase()}
          </p>
          <p
            className="mt-1 font-body text-sm"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {errored.message}
          </p>
        </div>
      ) : filtered.length === 0 ? (
        /* Empty state */
        <div
          className="rounded-xl border-dashed p-12 text-center"
          style={{ border: "2px dashed var(--thittam-border, #e2e8f0)" }}
        >
          <p
            className="font-heading text-lg font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            No {entityLabels.projectPlural.toLowerCase()} found
          </p>
          <p
            className="mt-1 font-body text-sm"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {search || filter !== "all"
              ? "Try adjusting your search or filters."
              : `Get started by creating your first ${entityLabels.project.toLowerCase()}.`}
          </p>
          {!search && filter === "all" && (
            <Link
              href="/productions/new"
              className="mt-4 inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <Plus className="h-4 w-4" />
              Create {entityLabels.project}
            </Link>
          )}
        </div>
      ) : viewMode === "grid" ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((p) => (
            <ProductionCard key={p.id} production={toCardProduction(p)} />
          ))}
        </div>
      ) : (
        /* Table view */
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr
                style={{
                  borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Title
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Status
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Genre
                </th>
                <th className="pb-2 pr-4 font-heading text-xs font-medium text-gray-500">
                  Start
                </th>
                <th className="pb-2 font-heading text-xs font-medium text-gray-500">
                  End
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((p) => (
                <tr
                  key={p.id}
                  className="group cursor-pointer hover:bg-gray-50/50"
                  style={{
                    borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                  }}
                >
                  <td className="py-3 pr-4">
                    <Link
                      href={`/productions/${p.id}`}
                      className="font-heading text-sm font-medium hover:underline"
                      style={{
                        color: "var(--thittam-foreground, #0f172a)",
                      }}
                    >
                      {p.title}
                    </Link>
                  </td>
                  <td className="py-3 pr-4">
                    <StatusBadge status={p.status} />
                  </td>
                  <td
                    className="py-3 pr-4 font-body text-sm"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {p.genre}
                  </td>
                  <td
                    className="py-3 pr-4 font-mono text-xs"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {p.start_date ?? "—"}
                  </td>
                  <td
                    className="py-3 font-mono text-xs"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {p.end_date ?? "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
