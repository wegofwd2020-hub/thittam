"use client";

import { useState } from "react";
import { Search, UserCog, ArrowUpCircle, Pause, Play } from "lucide-react";
import { StatusBadge } from "@/components/ui/status-badge";
import { PlanBadge } from "@/components/platform/plan-badge";
import { ImpersonationDialog } from "@/components/platform/impersonation-dialog";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

interface Tenant {
  id: string;
  name: string;
  slug: string;
  plan: string;
  vertical: string;
  status: string;
  userCount: number;
  createdAt: string;
  isDemo: boolean;
}

const mockTenants: Tenant[] = [
  {
    id: "t-001",
    name: "XYZ_CBA Productions",
    slug: "xyz-cba-prod",
    plan: "professional",
    vertical: "Movie Production",
    status: "active",
    userCount: 8,
    createdAt: "2025-09-14",
    isDemo: false,
  },
  {
    id: "t-002",
    name: "TechForge Solutions",
    slug: "techforge-sol",
    plan: "enterprise",
    vertical: "Software Development",
    status: "active",
    userCount: 12,
    createdAt: "2025-10-02",
    isDemo: false,
  },
  {
    id: "t-003",
    name: "BuildRight Engineers",
    slug: "buildright-eng",
    plan: "professional",
    vertical: "Construction",
    status: "active",
    userCount: 6,
    createdAt: "2025-11-18",
    isDemo: false,
  },
  {
    id: "t-004",
    name: "Grandeur Events",
    slug: "grandeur-events",
    plan: "starter",
    vertical: "Events Management",
    status: "active",
    userCount: 4,
    createdAt: "2026-01-05",
    isDemo: false,
  },
  {
    id: "t-005",
    name: "Demo Film Studio",
    slug: "demo-film-studio",
    plan: "professional",
    vertical: "Movie Production",
    status: "active",
    userCount: 8,
    createdAt: "2026-02-20",
    isDemo: true,
  },
  {
    id: "t-006",
    name: "Suspended Corp",
    slug: "suspended-corp",
    plan: "starter",
    vertical: "Software Development",
    status: "suspended",
    userCount: 2,
    createdAt: "2025-08-30",
    isDemo: false,
  },
];

// Mock users for impersonation dialog
const mockUsers = [
  { id: "u-001", name: "Alex Rivera", email: "alex@example.com", role: "owner" },
  { id: "u-002", name: "Jordan Kim", email: "jordan@example.com", role: "admin" },
  { id: "u-003", name: "Sam Patel", email: "sam@example.com", role: "member" },
  { id: "u-004", name: "Taylor Chen", email: "taylor@example.com", role: "viewer" },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function TenantsPage() {
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "suspended">("all");
  const [impersonateTenant, setImpersonateTenant] = useState<Tenant | null>(null);

  const filtered = mockTenants.filter((t) => {
    if (statusFilter !== "all" && t.status !== statusFilter) return false;
    if (
      search &&
      !t.name.toLowerCase().includes(search.toLowerCase()) &&
      !t.slug.toLowerCase().includes(search.toLowerCase()) &&
      !t.vertical.toLowerCase().includes(search.toLowerCase())
    ) {
      return false;
    }
    return true;
  });

  function handleImpersonateConfirm(userId: string, reason: string) {
    // In production this would call the platform API
    // eslint-disable-next-line no-console
    console.log("Impersonation started:", {
      tenantId: impersonateTenant?.id,
      userId,
      reason,
    });
    setImpersonateTenant(null);
  }

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6">
        <h1 className="font-heading text-2xl font-semibold tracking-tight text-slate-900">
          Tenant Management
        </h1>
        <p className="mt-1 font-body text-sm text-slate-500">
          View, manage, and impersonate tenant accounts across the platform.
        </p>
      </div>

      {/* Toolbar */}
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative flex-1 sm:max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Search tenants..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-lg border border-slate-300 bg-white py-2 pl-9 pr-3 font-body text-sm text-slate-900 placeholder:text-slate-400 focus:border-orange-500 focus:outline-none focus:ring-1 focus:ring-orange-500"
          />
        </div>

        <div className="flex items-center gap-1 rounded-lg bg-slate-100 p-0.5">
          {(["all", "active", "suspended"] as const).map((key) => (
            <button
              key={key}
              onClick={() => setStatusFilter(key)}
              className={`rounded-md px-3 py-1 font-heading text-xs font-medium transition-colors ${
                statusFilter === key
                  ? "bg-white text-slate-900 shadow-sm"
                  : "text-slate-500 hover:text-slate-700"
              }`}
            >
              {key.charAt(0).toUpperCase() + key.slice(1)}
            </button>
          ))}
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 bg-slate-50">
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Name
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Slug
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Plan
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Vertical
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Status
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Users
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Created
              </th>
              <th className="px-4 py-3 font-heading text-xs font-medium uppercase tracking-wider text-slate-500">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((tenant) => (
              <tr
                key={tenant.id}
                className="border-b border-slate-100 last:border-b-0 hover:bg-slate-50/50"
              >
                {/* Name */}
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <span className="font-heading text-sm font-medium text-slate-900">
                      {tenant.name}
                    </span>
                    {tenant.isDemo && (
                      <span className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-700">
                        DEMO
                      </span>
                    )}
                  </div>
                </td>

                {/* Slug */}
                <td className="px-4 py-3">
                  <span className="font-mono text-sm text-slate-600">
                    {tenant.slug}
                  </span>
                </td>

                {/* Plan */}
                <td className="px-4 py-3">
                  <PlanBadge plan={tenant.plan} />
                </td>

                {/* Vertical */}
                <td className="px-4 py-3">
                  <span className="font-body text-sm text-slate-600">
                    {tenant.vertical}
                  </span>
                </td>

                {/* Status */}
                <td className="px-4 py-3">
                  <StatusBadge status={tenant.status} />
                </td>

                {/* User count */}
                <td className="px-4 py-3">
                  <span className="font-mono text-sm text-slate-600">
                    {tenant.userCount}
                  </span>
                </td>

                {/* Created */}
                <td className="px-4 py-3">
                  <span className="font-mono text-sm text-slate-500">
                    {tenant.createdAt}
                  </span>
                </td>

                {/* Actions */}
                <td className="px-4 py-3">
                  <div className="flex items-center gap-1">
                    {/* Suspend / Reactivate */}
                    {tenant.status === "active" ? (
                      <button
                        className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-red-50 hover:text-red-600"
                        title="Suspend tenant"
                      >
                        <Pause className="h-4 w-4" />
                      </button>
                    ) : (
                      <button
                        className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-green-50 hover:text-green-600"
                        title="Reactivate tenant"
                      >
                        <Play className="h-4 w-4" />
                      </button>
                    )}

                    {/* Upgrade Plan */}
                    <button
                      className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-blue-50 hover:text-blue-600"
                      title="Upgrade plan"
                    >
                      <ArrowUpCircle className="h-4 w-4" />
                    </button>

                    {/* Impersonate */}
                    <button
                      onClick={() => setImpersonateTenant(tenant)}
                      className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-orange-50 hover:text-orange-600"
                      title="Impersonate user"
                      disabled={tenant.status === "suspended"}
                    >
                      <UserCog className="h-4 w-4" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        {filtered.length === 0 && (
          <div className="px-4 py-12 text-center">
            <p className="font-heading text-sm font-medium text-slate-500">
              No tenants found
            </p>
            <p className="mt-1 font-body text-xs text-slate-400">
              Try adjusting your search or filter.
            </p>
          </div>
        )}
      </div>

      {/* Summary */}
      <div className="mt-4 flex items-center justify-between">
        <p className="font-body text-xs text-slate-500">
          Showing {filtered.length} of {mockTenants.length} tenants
        </p>
      </div>

      {/* Impersonation dialog */}
      {impersonateTenant && (
        <ImpersonationDialog
          tenant={{
            id: impersonateTenant.id,
            name: impersonateTenant.name,
          }}
          users={mockUsers}
          isOpen={true}
          onClose={() => setImpersonateTenant(null)}
          onConfirm={handleImpersonateConfirm}
        />
      )}
    </div>
  );
}
