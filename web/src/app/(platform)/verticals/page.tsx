"use client";

import { useState } from "react";
import {
  Upload,
  Clapperboard,
  Server,
  HardHat,
  CalendarDays,
  AlertTriangle,
  type LucideIcon,
} from "lucide-react";
import { StatusBadge } from "@/components/ui/status-badge";
import { YAMLUploader } from "@/components/platform/yaml-uploader";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

interface VerticalDef {
  id: string;
  name: string;
  slug: string;
  version: string;
  status: string;
  tenantCount: number;
  themeColor: string;
  icon: LucideIcon;
  description: string;
}

const mockVerticals: VerticalDef[] = [
  {
    id: "v-001",
    name: "Movie Production",
    slug: "movie-production",
    version: "2.1.0",
    status: "active",
    tenantCount: 5,
    themeColor: "#6366f1",
    icon: Clapperboard,
    description:
      "Film and TV production management with phases like development, pre-production, production, post-production, and distribution.",
  },
  {
    id: "v-002",
    name: "Software Development",
    slug: "software-development",
    version: "1.4.0",
    status: "active",
    tenantCount: 3,
    themeColor: "#3b82f6",
    icon: Server,
    description:
      "Software project management with sprints, releases, and engineering-specific budget categories.",
  },
  {
    id: "v-003",
    name: "Construction",
    slug: "construction",
    version: "1.2.0",
    status: "active",
    tenantCount: 2,
    themeColor: "#f59e0b",
    icon: HardHat,
    description:
      "Construction project management with phases like planning, permitting, foundation, framing, and finishing.",
  },
  {
    id: "v-004",
    name: "Events Management",
    slug: "events-management",
    version: "1.0.0",
    status: "active",
    tenantCount: 2,
    themeColor: "#10b981",
    icon: CalendarDays,
    description:
      "Event planning and execution with venue management, vendor coordination, and day-of logistics.",
  },
];

// ---------------------------------------------------------------------------
// Deprecation confirmation dialog
// ---------------------------------------------------------------------------

function DeprecateDialog({
  vertical,
  onConfirm,
  onCancel,
}: {
  vertical: VerticalDef;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onCancel} />
      <div className="relative z-10 w-full max-w-md rounded-xl bg-white p-6 shadow-2xl">
        <div className="mb-4 flex items-center gap-2">
          <AlertTriangle className="h-5 w-5 text-red-500" />
          <h3 className="font-heading text-lg font-semibold text-slate-900">
            Deprecate Vertical
          </h3>
        </div>
        <p className="mb-2 font-body text-sm text-slate-600">
          Are you sure you want to deprecate{" "}
          <strong>{vertical.name}</strong>?
        </p>
        <p className="mb-6 font-body text-sm text-slate-500">
          This vertical is currently used by{" "}
          <span className="font-mono font-medium text-slate-700">
            {vertical.tenantCount}
          </span>{" "}
          tenant(s). Existing tenants will continue to function, but no new
          tenants can select this vertical.
        </p>
        <div className="flex items-center justify-end gap-3">
          <button
            onClick={onCancel}
            className="rounded-lg px-4 py-2 font-heading text-sm font-medium text-slate-700 hover:bg-slate-100"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="rounded-lg bg-red-600 px-4 py-2 font-heading text-sm font-medium text-white shadow-sm hover:bg-red-700"
          >
            Deprecate
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function VerticalsPage() {
  const [showUploader, setShowUploader] = useState(false);
  const [deprecateTarget, setDeprecateTarget] = useState<VerticalDef | null>(null);
  const [validationResult, setValidationResult] = useState<{
    valid: boolean;
    errors?: string[];
    summary?: { name: string; version: string };
  } | null>(null);

  function handleDeprecateConfirm() {
    // In production this would call the platform API
    // eslint-disable-next-line no-console
    console.log("Deprecated vertical:", deprecateTarget?.id);
    setDeprecateTarget(null);
  }

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-heading text-2xl font-semibold tracking-tight text-slate-900">
            Vertical Definitions
          </h1>
          <p className="mt-1 font-body text-sm text-slate-500">
            Manage industry vertical configurations. Each vertical defines
            entity labels, phases, budget categories, and theme settings.
          </p>
        </div>
        <button
          onClick={() => setShowUploader((prev) => !prev)}
          className="inline-flex items-center gap-2 rounded-lg bg-orange-600 px-4 py-2 font-heading text-sm font-medium text-white shadow-sm transition-colors hover:bg-orange-700"
        >
          <Upload className="h-4 w-4" />
          Upload YAML
        </button>
      </div>

      {/* Upload section */}
      {showUploader && (
        <div className="mb-8 rounded-xl border border-slate-200 bg-white p-6">
          <h2 className="mb-4 font-heading text-base font-semibold text-slate-900">
            Upload Vertical Definition
          </h2>
          <YAMLUploader
            onValidated={(result) =>
              setValidationResult({
                valid: result.valid,
                errors: result.errors,
                summary: result.summary
                  ? { name: result.summary.name, version: result.summary.version }
                  : undefined,
              })
            }
          />
          {validationResult?.valid && (
            <div className="mt-4 flex items-center justify-end">
              <button className="rounded-lg bg-green-600 px-4 py-2 font-heading text-sm font-medium text-white shadow-sm transition-colors hover:bg-green-700">
                Register Vertical
              </button>
            </div>
          )}
        </div>
      )}

      {/* Verticals grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-2">
        {mockVerticals.map((v) => {
          const Icon = v.icon;
          return (
            <div
              key={v.id}
              className="relative overflow-hidden rounded-xl border border-slate-200 bg-white"
            >
              {/* Theme color strip */}
              <div
                className="h-1.5"
                style={{ backgroundColor: v.themeColor }}
              />

              <div className="p-5">
                {/* Title row */}
                <div className="mb-3 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className="flex h-10 w-10 items-center justify-center rounded-lg"
                      style={{
                        backgroundColor: `${v.themeColor}15`,
                        color: v.themeColor,
                      }}
                    >
                      <Icon className="h-5 w-5" />
                    </div>
                    <div>
                      <h3 className="font-heading text-base font-semibold text-slate-900">
                        {v.name}
                      </h3>
                      <div className="mt-0.5 flex items-center gap-2">
                        <span className="font-mono text-xs text-slate-500">
                          v{v.version}
                        </span>
                        <StatusBadge status={v.status} />
                      </div>
                    </div>
                  </div>
                </div>

                {/* Description */}
                <p className="mb-4 font-body text-sm leading-relaxed text-slate-600">
                  {v.description}
                </p>

                {/* Stats + actions */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div>
                      <p className="font-heading text-xs font-medium uppercase tracking-wider text-slate-400">
                        Tenants
                      </p>
                      <p className="font-mono text-lg font-semibold text-slate-900">
                        {v.tenantCount}
                      </p>
                    </div>
                    <div>
                      <p className="font-heading text-xs font-medium uppercase tracking-wider text-slate-400">
                        Theme
                      </p>
                      <div className="mt-1 flex items-center gap-1.5">
                        <div
                          className="h-4 w-4 rounded-full border border-slate-200"
                          style={{ backgroundColor: v.themeColor }}
                        />
                        <span className="font-mono text-xs text-slate-500">
                          {v.themeColor}
                        </span>
                      </div>
                    </div>
                  </div>

                  <button
                    onClick={() => setDeprecateTarget(v)}
                    className="rounded-lg border border-red-200 px-3 py-1.5 font-heading text-xs font-medium text-red-600 transition-colors hover:bg-red-50"
                  >
                    Deprecate
                  </button>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Deprecation dialog */}
      {deprecateTarget && (
        <DeprecateDialog
          vertical={deprecateTarget}
          onConfirm={handleDeprecateConfirm}
          onCancel={() => setDeprecateTarget(null)}
        />
      )}
    </div>
  );
}
