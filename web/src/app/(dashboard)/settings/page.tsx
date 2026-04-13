"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";
import { PageHeader } from "@/components/ui/page-header";
import { StatusBadge } from "@/components/ui/status-badge";
import { PlanBadge } from "@/components/platform/plan-badge";
import { DataTable, type Column } from "@/components/ui/data-table";
import { InviteUserDialog } from "@/components/settings/invite-user-dialog";
import { RolePermissionMatrix } from "@/components/settings/role-permission-matrix";
import { OidcConfigForm } from "@/components/settings/oidc-config-form";
import { AuditLogViewer, type AuditEntry } from "@/components/settings/audit-log-viewer";
import { ThemeCustomizer } from "@/components/settings/theme-customizer";

// ---------------------------------------------------------------------------
// Tab definitions
// ---------------------------------------------------------------------------

const TABS = [
  { key: "users", label: "Users" },
  { key: "roles", label: "Roles" },
  { key: "auth", label: "Authentication" },
  { key: "audit", label: "Audit Log" },
  { key: "tenant", label: "Tenant Info" },
  { key: "appearance", label: "Appearance" },
] as const;

type TabKey = (typeof TABS)[number]["key"];

// ---------------------------------------------------------------------------
// Mock data — Users (8 users matching XYZ_CBA production company)
// ---------------------------------------------------------------------------

interface MockUser {
  id: string;
  email: string;
  display_name: string;
  status: string;
  roles: string[];
  last_login_at: string | null;
  created_at: string;
}

const mockUsers: MockUser[] = [
  {
    id: "usr-001",
    email: "rajesh.kumar@xyzcba.com",
    display_name: "Rajesh Kumar",
    status: "active",
    roles: ["super_admin"],
    last_login_at: "2026-04-04T08:15:00Z",
    created_at: "2025-06-01T10:00:00Z",
  },
  {
    id: "usr-002",
    email: "priya.sharma@xyzcba.com",
    display_name: "Priya Sharma",
    status: "active",
    roles: ["manager"],
    last_login_at: "2026-04-04T09:30:00Z",
    created_at: "2025-06-15T14:00:00Z",
  },
  {
    id: "usr-003",
    email: "arjun.mehta@xyzcba.com",
    display_name: "Arjun Mehta",
    status: "active",
    roles: ["coordinator"],
    last_login_at: "2026-04-03T17:45:00Z",
    created_at: "2025-07-01T09:00:00Z",
  },
  {
    id: "usr-004",
    email: "deepa.nair@xyzcba.com",
    display_name: "Deepa Nair",
    status: "active",
    roles: ["accountant"],
    last_login_at: "2026-04-04T07:00:00Z",
    created_at: "2025-07-10T11:00:00Z",
  },
  {
    id: "usr-005",
    email: "kiran.rao@xyzcba.com",
    display_name: "Kiran Rao",
    status: "active",
    roles: ["production_manager"],
    last_login_at: "2026-04-03T19:20:00Z",
    created_at: "2025-08-05T10:00:00Z",
  },
  {
    id: "usr-006",
    email: "ravi.kumar@xyzcba.com",
    display_name: "Ravi Kumar",
    status: "active",
    roles: ["member"],
    last_login_at: "2026-04-02T14:10:00Z",
    created_at: "2025-09-01T08:00:00Z",
  },
  {
    id: "usr-007",
    email: "meena.iyer@xyzcba.com",
    display_name: "Meena Iyer",
    status: "invited",
    roles: ["production_manager"],
    last_login_at: null,
    created_at: "2026-03-28T16:00:00Z",
  },
  {
    id: "usr-008",
    email: "vikram.singh@xyzcba.com",
    display_name: "Vikram Singh",
    status: "deactivated",
    roles: ["member"],
    last_login_at: "2025-12-15T11:30:00Z",
    created_at: "2025-08-20T09:00:00Z",
  },
];

// ---------------------------------------------------------------------------
// Mock data — Roles with permissions
// ---------------------------------------------------------------------------

const mockRoles = [
  {
    id: "role-001",
    name: "super_admin",
    permissions: [
      "production:read", "production:write",
      "budget:read", "budget:write", "budget:approve",
      "expense:read", "expense:write", "expense:approve",
      "inventory:read", "inventory:write",
      "report:read",
      "user:manage",
    ],
  },
  {
    id: "role-002",
    name: "manager",
    permissions: [
      "production:read", "production:write",
      "budget:read", "budget:write", "budget:approve",
      "expense:read", "expense:approve",
      "inventory:read",
      "report:read",
      "user:manage",
    ],
  },
  {
    id: "role-003",
    name: "coordinator",
    permissions: [
      "production:read", "production:write",
      "budget:read", "budget:write",
      "expense:read", "expense:write", "expense:approve",
      "inventory:read", "inventory:write",
      "report:read",
    ],
  },
  {
    id: "role-004",
    name: "accountant",
    permissions: [
      "production:read",
      "budget:read", "budget:write",
      "expense:read", "expense:write", "expense:approve",
      "inventory:read",
      "report:read",
    ],
  },
  {
    id: "role-005",
    name: "production_manager",
    permissions: [
      "production:read", "production:write",
      "budget:read",
      "expense:read", "expense:write",
      "inventory:read", "inventory:write",
      "report:read",
    ],
  },
  {
    id: "role-006",
    name: "member",
    permissions: [
      "production:read",
      "budget:read",
      "expense:read", "expense:write",
      "inventory:read",
    ],
  },
];

// ---------------------------------------------------------------------------
// Mock data — Audit log (10 entries)
// ---------------------------------------------------------------------------

const mockAuditEntries: AuditEntry[] = [
  {
    id: "aud-001",
    actor_email: "rajesh.kumar@xyzcba.com",
    actor_ip: "103.21.244.12",
    action: "login_success",
    resource_type: "session",
    resource_id: "sess-a1b2c3d4",
    old_state: null,
    new_state: { method: "local", user_agent: "Chrome/122" },
    occurred_at: "2026-04-04T08:15:00Z",
  },
  {
    id: "aud-002",
    actor_email: "deepa.nair@xyzcba.com",
    actor_ip: "103.21.244.15",
    action: "approved",
    resource_type: "expense",
    resource_id: "exp-005",
    old_state: { status: "submitted", amount: "750000.00" },
    new_state: { status: "approved", amount: "750000.00", approved_by: "usr-004" },
    occurred_at: "2026-04-03T16:30:00Z",
  },
  {
    id: "aud-003",
    actor_email: "priya.sharma@xyzcba.com",
    actor_ip: "103.21.244.18",
    action: "approved",
    resource_type: "budget",
    resource_id: "bdgt-ver-003",
    old_state: { status: "submitted", total: "15000000.00" },
    new_state: { status: "approved", total: "15000000.00", approved_by: "usr-002" },
    occurred_at: "2026-04-03T14:20:00Z",
  },
  {
    id: "aud-004",
    actor_email: "arjun.mehta@xyzcba.com",
    actor_ip: "103.21.244.20",
    action: "created",
    resource_type: "expense",
    resource_id: "exp-008",
    old_state: null,
    new_state: { description: "Drone Operator Package", amount: "180000.00", status: "submitted" },
    occurred_at: "2026-04-03T11:00:00Z",
  },
  {
    id: "aud-005",
    actor_email: "rajesh.kumar@xyzcba.com",
    actor_ip: "103.21.244.12",
    action: "updated",
    resource_type: "user",
    resource_id: "usr-007",
    old_state: { roles: ["member"] },
    new_state: { roles: ["production_manager"] },
    occurred_at: "2026-04-02T15:45:00Z",
  },
  {
    id: "aud-006",
    actor_email: "kiran.rao@xyzcba.com",
    actor_ip: "103.21.244.22",
    action: "updated",
    resource_type: "production",
    resource_id: "prod-001",
    old_state: { phase: "pre_production" },
    new_state: { phase: "production" },
    occurred_at: "2026-04-01T10:00:00Z",
  },
  {
    id: "aud-007",
    actor_email: "deepa.nair@xyzcba.com",
    actor_ip: "103.21.244.15",
    action: "rejected",
    resource_type: "expense",
    resource_id: "exp-010",
    old_state: { status: "submitted", amount: "3500000.00" },
    new_state: { status: "rejected", reason: "Over budget — IMAX not approved for this production" },
    occurred_at: "2026-03-30T09:15:00Z",
  },
  {
    id: "aud-008",
    actor_email: "vikram.singh@xyzcba.com",
    actor_ip: "103.21.244.30",
    action: "login_failed",
    resource_type: "session",
    resource_id: "sess-failed-001",
    old_state: null,
    new_state: { reason: "invalid_password", attempts: 3 },
    occurred_at: "2026-03-28T22:10:00Z",
  },
  {
    id: "aud-009",
    actor_email: "priya.sharma@xyzcba.com",
    actor_ip: "103.21.244.18",
    action: "created",
    resource_type: "purchase_order",
    resource_id: "po-005",
    old_state: null,
    new_state: { vendor: "SkyVision Drones", amount: "180000.00", status: "draft" },
    occurred_at: "2026-03-27T14:00:00Z",
  },
  {
    id: "aud-010",
    actor_email: "rajesh.kumar@xyzcba.com",
    actor_ip: "103.21.244.12",
    action: "created",
    resource_type: "user",
    resource_id: "usr-007",
    old_state: null,
    new_state: { email: "meena.iyer@xyzcba.com", roles: ["member"], status: "invited" },
    occurred_at: "2026-03-28T16:00:00Z",
  },
];

// ---------------------------------------------------------------------------
// Mock data — Tenant settings
// ---------------------------------------------------------------------------

const mockTenant = {
  name: "XYZ CBA Productions",
  slug: "xyz-cba",
  plan: "professional",
  vertical_id: "movie-production",
  vertical_name: "Movie Production",
  created_at: "2025-06-01T10:00:00Z",
};

// ---------------------------------------------------------------------------
// Mock data — Auth config
// ---------------------------------------------------------------------------

const mockAuthConfig = {
  auth_method: "local" as string,
  issuer_url: null as string | null,
  client_id: null as string | null,
  scopes: ["openid", "email", "profile"],
  email_claim: "email",
  display_name_claim: "name",
  groups_claim: null as string | null,
  auto_provision: false,
  default_role: "member",
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatRoleName(name: string): string {
  return name.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

function formatDateTime(iso: string | null): string {
  if (!iso) return "\u2014";
  return new Date(iso).toLocaleString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function SettingsPage() {
  const { entityLabels } = useTheme();
  const [activeTab, setActiveTab] = useState<TabKey>("users");
  const [showInvite, setShowInvite] = useState(false);
  const [oidcEnabled, setOidcEnabled] = useState(
    mockAuthConfig.auth_method === "oidc",
  );

  // Users table columns
  const userColumns: Column<MockUser>[] = [
    {
      key: "display_name",
      header: "Name",
      sortable: true,
      render: (row) => (
        <span
          className="font-heading text-sm font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.display_name}
        </span>
      ),
    },
    {
      key: "email",
      header: "Email",
      sortable: true,
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.email}
        </span>
      ),
    },
    {
      key: "roles",
      header: "Roles",
      render: (row) => (
        <div className="flex flex-wrap gap-1">
          {row.roles.map((r) => (
            <span
              key={r}
              className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
              style={{
                backgroundColor:
                  r === "super_admin"
                    ? "#fae8ff"
                    : r === "manager"
                      ? "#dbeafe"
                      : "var(--thittam-muted, #f1f5f9)",
                color:
                  r === "super_admin"
                    ? "#a21caf"
                    : r === "manager"
                      ? "#1d4ed8"
                      : "var(--thittam-foreground, #0f172a)",
              }}
            >
              {formatRoleName(r)}
            </span>
          ))}
        </div>
      ),
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: "last_login_at",
      header: "Last Login",
      type: "date",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {formatDateTime(row.last_login_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <div className="flex items-center gap-2">
          {row.status !== "deactivated" && (
            <>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  toast.success(`Role edit dialog for ${row.display_name}`);
                }}
                className="text-xs font-heading font-medium transition-colors hover:opacity-80"
                style={{ color: "var(--thittam-primary, #3b82f6)" }}
              >
                Edit Role
              </button>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  if (
                    window.confirm(
                      `Are you sure you want to deactivate ${row.display_name}? This action cannot be undone.`,
                    )
                  ) {
                    toast.success(`${row.display_name} deactivated`);
                  }
                }}
                className="text-xs font-heading font-medium transition-colors hover:opacity-80 text-red-600"
              >
                Deactivate
              </button>
            </>
          )}
        </div>
      ),
    },
  ];

  // Unique actors for audit filter
  const auditActors = Array.from(
    new Set(mockAuditEntries.map((e) => e.actor_email)),
  );

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title="Settings"
        description="Manage users, roles, authentication, and tenant configuration"
        icon="Settings"
      />

      {/* Tab navigation */}
      <div
        className="mb-6 flex border-b"
        style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
      >
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2.5 text-sm font-heading font-medium transition-colors ${
              activeTab === tab.key ? "border-b-2" : ""
            }`}
            style={
              activeTab === tab.key
                ? {
                    borderColor: "var(--thittam-primary, #3b82f6)",
                    color: "var(--thittam-primary, #3b82f6)",
                  }
                : {
                    color: "var(--thittam-muted-foreground, #64748b)",
                  }
            }
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* ================================================================= */}
      {/* Users Tab                                                         */}
      {/* ================================================================= */}
      {activeTab === "users" && (
        <div>
          <div className="mb-4 flex items-center justify-between">
            <h2
              className="font-heading text-lg font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Team Members
            </h2>
            <button
              onClick={() => setShowInvite(true)}
              className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              <Plus className="h-4 w-4" />
              Invite User
            </button>
          </div>

          <DataTable<Record<string, unknown>>
            columns={userColumns as unknown as Column<Record<string, unknown>>[]}
            data={mockUsers as unknown as Record<string, unknown>[]}
            emptyMessage="No users found."
          />

          {showInvite && (
            <InviteUserDialog
              roles={mockRoles}
              onConfirm={(data) => {
                toast.success(`Invitation sent to ${data.email}`);
                setShowInvite(false);
              }}
              onCancel={() => setShowInvite(false)}
            />
          )}
        </div>
      )}

      {/* ================================================================= */}
      {/* Roles Tab                                                         */}
      {/* ================================================================= */}
      {activeTab === "roles" && (
        <div>
          <div className="mb-4">
            <h2
              className="font-heading text-lg font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Role-Permission Matrix
            </h2>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Overview of which permissions are granted to each role. Roles are
              defined at deployment time and managed via configuration.
            </p>
          </div>

          <RolePermissionMatrix roles={mockRoles} />
        </div>
      )}

      {/* ================================================================= */}
      {/* Authentication Tab                                                */}
      {/* ================================================================= */}
      {activeTab === "auth" && (
        <div>
          <div className="mb-6">
            <h2
              className="font-heading text-lg font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Authentication Configuration
            </h2>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Configure how users authenticate with your tenant.
            </p>
          </div>

          {/* Current method badge */}
          <div
            className="mb-6 rounded-xl p-5"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <div className="flex items-center justify-between">
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Current Method
                </span>
                <div className="mt-1 flex items-center gap-2">
                  <StatusBadge
                    status={oidcEnabled ? "active" : "active"}
                  />
                  <span
                    className="font-heading text-sm font-medium"
                    style={{ color: "var(--thittam-foreground, #0f172a)" }}
                  >
                    {oidcEnabled
                      ? "OIDC (Single Sign-On)"
                      : "Local (Email/Password)"}
                  </span>
                </div>
              </div>

              {/* OIDC toggle */}
              <div className="flex items-center gap-3">
                <span
                  className="text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Enable OIDC
                </span>
                <button
                  type="button"
                  role="switch"
                  aria-checked={oidcEnabled}
                  onClick={() => setOidcEnabled(!oidcEnabled)}
                  className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full transition-colors ${
                    oidcEnabled ? "" : "bg-gray-200"
                  }`}
                  style={
                    oidcEnabled
                      ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
                      : {}
                  }
                >
                  <span
                    className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-sm ring-0 transition-transform ${
                      oidcEnabled ? "translate-x-5" : "translate-x-0.5"
                    } mt-0.5`}
                  />
                </button>
              </div>
            </div>
          </div>

          {/* OIDC config form (shown when enabled) */}
          {oidcEnabled && (
            <div
              className="rounded-xl p-5"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <OidcConfigForm
                initialValues={{
                  issuer_url: mockAuthConfig.issuer_url ?? "",
                  client_id: mockAuthConfig.client_id ?? "",
                  client_secret: "",
                  scopes: mockAuthConfig.scopes,
                  email_claim: mockAuthConfig.email_claim,
                  display_name_claim: mockAuthConfig.display_name_claim,
                  groups_claim: mockAuthConfig.groups_claim ?? "",
                  auto_provision: mockAuthConfig.auto_provision,
                  default_role: mockAuthConfig.default_role,
                }}
                roles={mockRoles}
                onSave={(values) => {
                  toast.success("OIDC configuration saved");
                }}
              />
            </div>
          )}
        </div>
      )}

      {/* ================================================================= */}
      {/* Audit Log Tab                                                     */}
      {/* ================================================================= */}
      {activeTab === "audit" && (
        <div>
          <div className="mb-4">
            <h2
              className="font-heading text-lg font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Audit Log
            </h2>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Immutable record of all security and administrative events.
              Entries cannot be modified or deleted.
            </p>
          </div>

          <AuditLogViewer
            entries={mockAuditEntries}
            actors={auditActors}
          />
        </div>
      )}

      {/* ================================================================= */}
      {/* Tenant Info Tab                                                   */}
      {/* ================================================================= */}
      {activeTab === "tenant" && (
        <div>
          <div className="mb-4">
            <h2
              className="font-heading text-lg font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Tenant Information
            </h2>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Your organization details and subscription.
            </p>
          </div>

          <div
            className="rounded-xl p-6"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
              {/* Company name */}
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Company Name
                </span>
                <span
                  className="mt-1 block font-heading text-sm font-semibold"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {mockTenant.name}
                </span>
              </div>

              {/* Slug */}
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Slug
                </span>
                <span
                  className="mt-1 block font-mono tabular-nums text-sm"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {mockTenant.slug}
                </span>
              </div>

              {/* Plan */}
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Plan
                </span>
                <div className="mt-1">
                  <PlanBadge plan={mockTenant.plan} />
                </div>
              </div>

              {/* Vertical name */}
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Vertical
                </span>
                <span
                  className="mt-1 block font-heading text-sm font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {mockTenant.vertical_name}
                </span>
              </div>

              {/* Vertical ID */}
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Vertical ID
                </span>
                <span
                  className="mt-1 block font-mono tabular-nums text-sm"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {mockTenant.vertical_id}
                </span>
              </div>

              {/* Created date */}
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Created
                </span>
                <span
                  className="mt-1 block font-mono tabular-nums text-sm"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {formatDate(mockTenant.created_at)}
                </span>
              </div>
            </div>

            {/* Billing link */}
            <div
              className="mt-6 rounded-lg px-4 py-3 text-xs font-body"
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                color: "var(--thittam-muted-foreground, #64748b)",
              }}
            >
              To manage your subscription, view invoices, or update payment methods, go to{" "}
              <a
                href="/billing"
                className="font-medium underline"
                style={{ color: "var(--thittam-primary, #3b82f6)" }}
              >
                Billing
              </a>
              . To change your vertical configuration, contact{" "}
              <a
                href="mailto:support@thittam.io"
                className="font-medium underline"
                style={{ color: "var(--thittam-primary, #3b82f6)" }}
              >
                support@thittam.io
              </a>
              .
            </div>
          </div>
        </div>
      )}

      {/* ================================================================= */}
      {/* Appearance Tab                                                    */}
      {/* ================================================================= */}
      {activeTab === "appearance" && (
        <div>
          <div className="mb-6">
            <h2
              className="font-heading text-lg font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Theme & Appearance
            </h2>
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Customize colors, branding, and typography for your organization.
              Changes are previewed in real time.
            </p>
          </div>

          <ThemeCustomizer />
        </div>
      )}
    </div>
  );
}
