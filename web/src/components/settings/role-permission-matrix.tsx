"use client";

import { Check } from "lucide-react";

// ---------------------------------------------------------------------------
// RolePermissionMatrix — read-only matrix of roles vs permissions.
// ---------------------------------------------------------------------------

interface RolePermissionMatrixProps {
  roles: { name: string; permissions: string[] }[];
}

const ALL_PERMISSIONS = [
  "production:read",
  "production:write",
  "budget:read",
  "budget:write",
  "budget:approve",
  "expense:read",
  "expense:write",
  "expense:approve",
  "inventory:read",
  "inventory:write",
  "report:read",
  "user:manage",
] as const;

function formatPermission(perm: string): string {
  const [resource, action] = perm.split(":");
  return `${action.charAt(0).toUpperCase()}${action.slice(1)}`;
}

function formatResource(perm: string): string {
  const [resource] = perm.split(":");
  return resource.charAt(0).toUpperCase() + resource.slice(1);
}

function formatRoleName(name: string): string {
  return name.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

// Group permissions by resource for header display
function getResourceGroups(): { resource: string; permissions: string[] }[] {
  const groups: { resource: string; permissions: string[] }[] = [];
  let current: { resource: string; permissions: string[] } | null = null;

  for (const perm of ALL_PERMISSIONS) {
    const resource = perm.split(":")[0];
    if (!current || current.resource !== resource) {
      current = { resource, permissions: [] };
      groups.push(current);
    }
    current.permissions.push(perm);
  }

  return groups;
}

export function RolePermissionMatrix({ roles }: RolePermissionMatrixProps) {
  const groups = getResourceGroups();

  return (
    <div
      className="overflow-x-auto rounded-lg border"
      style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
    >
      <table className="w-full text-sm">
        <thead>
          {/* Resource group row */}
          <tr
            style={{
              backgroundColor: "var(--thittam-muted, #f1f5f9)",
              borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <th
              className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wider font-heading"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              rowSpan={2}
            >
              Role
            </th>
            {groups.map((g) => (
              <th
                key={g.resource}
                colSpan={g.permissions.length}
                className="px-2 py-2 text-center text-xs font-semibold uppercase tracking-wider font-heading"
                style={{
                  color: "var(--thittam-foreground, #0f172a)",
                  borderLeft: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                {formatResource(g.permissions[0])}
              </th>
            ))}
          </tr>
          {/* Action row */}
          <tr
            style={{
              backgroundColor: "var(--thittam-muted, #f1f5f9)",
              borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            {ALL_PERMISSIONS.map((perm, idx) => {
              const isGroupStart = idx === 0 || perm.split(":")[0] !== ALL_PERMISSIONS[idx - 1].split(":")[0];
              return (
                <th
                  key={perm}
                  className="px-2 py-2 text-center text-[10px] font-medium uppercase tracking-wider font-heading"
                  style={{
                    color: "var(--thittam-muted-foreground, #64748b)",
                    ...(isGroupStart
                      ? { borderLeft: "1px solid var(--thittam-border, #e2e8f0)" }
                      : {}),
                  }}
                >
                  {formatPermission(perm)}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {roles.map((role, idx) => (
            <tr
              key={role.name}
              className={idx % 2 === 1 ? "bg-gray-50/50" : ""}
              style={{
                borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <td
                className="px-4 py-3 font-body text-sm font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {formatRoleName(role.name)}
              </td>
              {ALL_PERMISSIONS.map((perm, permIdx) => {
                const hasPermission = role.permissions.includes(perm);
                const isGroupStart =
                  permIdx === 0 ||
                  perm.split(":")[0] !== ALL_PERMISSIONS[permIdx - 1].split(":")[0];
                return (
                  <td
                    key={perm}
                    className="px-2 py-3 text-center"
                    style={
                      isGroupStart
                        ? { borderLeft: "1px solid var(--thittam-border, #e2e8f0)" }
                        : {}
                    }
                  >
                    {hasPermission ? (
                      <Check className="mx-auto h-4 w-4 text-green-600" />
                    ) : null}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>

      {/* Read-only notice */}
      <div
        className="px-4 py-3 text-xs font-body"
        style={{
          backgroundColor: "var(--thittam-muted, #f1f5f9)",
          color: "var(--thittam-muted-foreground, #64748b)",
          borderTop: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        Role permissions are defined at deployment time and cannot be modified from the UI.
      </div>
    </div>
  );
}
