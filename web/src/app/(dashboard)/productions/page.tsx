"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { Plus, Search, LayoutGrid, List } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import {
  ProductionCard,
  type Production,
} from "@/components/productions/production-card";
import { StatusBadge } from "@/components/ui/status-badge";

// ---------------------------------------------------------------------------
// Mock data — will be replaced by API calls when the backend is ready
// ---------------------------------------------------------------------------
const mockProductions: Production[] = [
  {
    id: "1",
    title: "The Last Horizon",
    status: "production",
    description:
      "A sci-fi thriller following a crew of astronauts who discover an anomaly at the edge of the solar system that challenges everything they know about reality.",
    genre: "Sci-Fi",
    currentPhase: "production",
    budgetHealth: "on_track",
    startDate: "2026-01-15",
    endDate: "2026-08-30",
  },
  {
    id: "2",
    title: "Midnight Express Reboot",
    status: "post_production",
    description:
      "A modern reimagining of the classic thriller, set in contemporary Istanbul with a focus on digital-age surveillance and personal freedom.",
    genre: "Thriller",
    currentPhase: "post_production",
    budgetHealth: "at_risk",
    startDate: "2025-06-01",
    endDate: "2026-03-15",
  },
  {
    id: "3",
    title: "Project Starfall",
    status: "development",
    description:
      "An animated feature exploring a young girl's journey through a world where fallen stars grant wishes — but at a hidden cost.",
    genre: "Animation",
    currentPhase: "development",
    budgetHealth: "on_track",
    startDate: "2026-03-01",
  },
  {
    id: "4",
    title: "Urban Legends: Season 2",
    status: "pre_production",
    description:
      "The second season of the hit anthology series. Each episode brings a different urban legend to life with a fresh directorial voice.",
    genre: "Horror",
    currentPhase: "pre_production",
    budgetHealth: "on_track",
    startDate: "2026-04-10",
  },
  {
    id: "5",
    title: "The Color of Sound",
    status: "archived",
    description:
      "A documentary exploring the phenomenon of synesthesia through the eyes of five musicians who see color when they hear music.",
    genre: "Documentary",
    currentPhase: "released",
    budgetHealth: "over_budget",
    startDate: "2024-09-01",
    endDate: "2025-11-20",
  },
];

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

function matchesFilter(production: Production, filter: FilterKey): boolean {
  if (filter === "all") return true;
  if (filter === "active") return production.status !== "archived";
  return production.status === filter;
}

// ---------------------------------------------------------------------------
// Loading skeleton
// ---------------------------------------------------------------------------
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

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function ProductionsPage() {
  const { entityLabels } = useTheme();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<FilterKey>("all");
  const [viewMode, setViewMode] = useState<"grid" | "table">("grid");
  const [isLoading] = useState(false); // Toggle to true to preview skeleton

  const filtered = useMemo(() => {
    return mockProductions.filter((p) => {
      if (!matchesFilter(p, filter)) return false;
      if (
        search &&
        !p.title.toLowerCase().includes(search.toLowerCase()) &&
        !p.description.toLowerCase().includes(search.toLowerCase())
      ) {
        return false;
      }
      return true;
    });
  }, [search, filter]);

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

        <div className="flex items-center gap-1 rounded-lg p-0.5"
          style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
        >
          <button
            onClick={() => setViewMode("grid")}
            className={`rounded-md p-1.5 transition-colors ${
              viewMode === "grid" ? "bg-white shadow-sm" : ""
            }`}
            title="Grid view"
          >
            <LayoutGrid className="h-4 w-4" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
          </button>
          <button
            onClick={() => setViewMode("table")}
            className={`rounded-md p-1.5 transition-colors ${
              viewMode === "table" ? "bg-white shadow-sm" : ""
            }`}
            title="Table view"
          >
            <List className="h-4 w-4" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
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
            {search
              ? "Try adjusting your search or filters."
              : `Get started by creating your first ${entityLabels.project.toLowerCase()}.`}
          </p>
          {!search && (
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
            <ProductionCard key={p.id} production={p} />
          ))}
        </div>
      ) : (
        /* Table view */
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr style={{ borderBottom: "1px solid var(--thittam-border, #e2e8f0)" }}>
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
                  Phase
                </th>
                <th className="pb-2 font-heading text-xs font-medium text-gray-500">
                  Budget
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((p) => (
                <tr
                  key={p.id}
                  className="group cursor-pointer hover:bg-gray-50/50"
                  style={{ borderBottom: "1px solid var(--thittam-border, #e2e8f0)" }}
                >
                  <td className="py-3 pr-4">
                    <Link
                      href={`/productions/${p.id}`}
                      className="font-heading text-sm font-medium hover:underline"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {p.title}
                    </Link>
                  </td>
                  <td className="py-3 pr-4">
                    <StatusBadge status={p.status} />
                  </td>
                  <td
                    className="py-3 pr-4 font-body text-sm"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    {p.genre}
                  </td>
                  <td className="py-3 pr-4">
                    <StatusBadge status={p.currentPhase} variant="phase" />
                  </td>
                  <td className="py-3">
                    <StatusBadge status={p.budgetHealth} variant="health" />
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
