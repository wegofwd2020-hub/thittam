"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { StatusBadge } from "@/components/ui/status-badge";

// ---------------------------------------------------------------------------
// AuditLogViewer — filterable, expandable audit log table.
// ---------------------------------------------------------------------------

export interface AuditEntry {
  id: string;
  actor_email: string;
  actor_ip: string;
  action: string;
  resource_type: string;
  resource_id: string;
  old_state: unknown;
  new_state: unknown;
  occurred_at: string;
}

interface AuditLogViewerProps {
  entries: AuditEntry[];
  actors: string[];
}

const ACTION_OPTIONS = [
  "All Actions",
  "created",
  "updated",
  "approved",
  "rejected",
  "login_success",
  "login_failed",
];

const RESOURCE_TYPE_OPTIONS = [
  "All Resources",
  "expense",
  "budget",
  "purchase_order",
  "user",
  "production",
  "phase",
  "session",
];

// Action-specific badge colors — map actions to status badge equivalents
const ACTION_STATUS_MAP: Record<string, string> = {
  created: "active",
  updated: "submitted",
  approved: "approved",
  rejected: "rejected",
  login_success: "active",
  login_failed: "rejected",
};

function formatTimestamp(ts: string): string {
  const d = new Date(ts);
  return d.toLocaleString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function truncateId(id: string, maxLen = 12): string {
  if (id.length <= maxLen) return id;
  return `${id.slice(0, maxLen)}...`;
}

export function AuditLogViewer({ entries, actors }: AuditLogViewerProps) {
  const [actionFilter, setActionFilter] = useState("All Actions");
  const [resourceFilter, setResourceFilter] = useState("All Resources");
  const [actorFilter, setActorFilter] = useState("All Actors");
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const filtered = entries.filter((e) => {
    if (actionFilter !== "All Actions" && e.action !== actionFilter) return false;
    if (resourceFilter !== "All Resources" && e.resource_type !== resourceFilter)
      return false;
    if (actorFilter !== "All Actors" && e.actor_email !== actorFilter)
      return false;
    return true;
  });

  const selectStyle = {
    backgroundColor: "var(--thittam-background, #fff)",
    border: "1px solid var(--thittam-border, #e2e8f0)",
    color: "var(--thittam-foreground, #0f172a)",
  };

  return (
    <div className="flex flex-col gap-4">
      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <select
          value={actorFilter}
          onChange={(e) => setActorFilter(e.target.value)}
          className="rounded-lg px-3 py-1.5 text-sm font-heading"
          style={selectStyle}
        >
          <option>All Actors</option>
          {actors.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>

        <select
          value={actionFilter}
          onChange={(e) => setActionFilter(e.target.value)}
          className="rounded-lg px-3 py-1.5 text-sm font-heading"
          style={selectStyle}
        >
          {ACTION_OPTIONS.map((a) => (
            <option key={a} value={a}>
              {a === "All Actions"
                ? a
                : a.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())}
            </option>
          ))}
        </select>

        <select
          value={resourceFilter}
          onChange={(e) => setResourceFilter(e.target.value)}
          className="rounded-lg px-3 py-1.5 text-sm font-heading"
          style={selectStyle}
        >
          {RESOURCE_TYPE_OPTIONS.map((r) => (
            <option key={r} value={r}>
              {r === "All Resources"
                ? r
                : r.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())}
            </option>
          ))}
        </select>
      </div>

      {/* Table */}
      <div
        className="overflow-x-auto rounded-lg border"
        style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
      >
        <table className="w-full text-sm">
          <thead>
            <tr
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <th className="w-8 px-2 py-3" />
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Timestamp
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Actor
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Action
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Resource Type
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Resource ID
              </th>
              <th
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                IP
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 && (
              <tr>
                <td
                  colSpan={7}
                  className="px-4 py-12 text-center font-body"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  No audit log entries match the selected filters.
                </td>
              </tr>
            )}
            {filtered.map((entry, idx) => {
              const isExpanded = expandedId === entry.id;
              const statusKey = ACTION_STATUS_MAP[entry.action] ?? entry.action;

              return (
                <tbody key={entry.id}>
                  <tr
                    onClick={() => setExpandedId(isExpanded ? null : entry.id)}
                    className={`cursor-pointer transition-colors hover:bg-gray-100/70 ${
                      idx % 2 === 1 ? "bg-gray-50/50" : ""
                    }`}
                    style={{
                      borderBottom: isExpanded
                        ? "none"
                        : "1px solid var(--thittam-border, #e2e8f0)",
                    }}
                  >
                    <td className="px-2 py-3 text-center">
                      {isExpanded ? (
                        <ChevronDown
                          className="mx-auto h-4 w-4"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        />
                      ) : (
                        <ChevronRight
                          className="mx-auto h-4 w-4"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        />
                      )}
                    </td>
                    <td
                      className="px-4 py-3 font-mono tabular-nums text-xs"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {formatTimestamp(entry.occurred_at)}
                    </td>
                    <td
                      className="px-4 py-3 font-body text-sm"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {entry.actor_email}
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={statusKey} />
                    </td>
                    <td
                      className="px-4 py-3 font-heading text-xs"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {entry.resource_type
                        .replace(/_/g, " ")
                        .replace(/\b\w/g, (c) => c.toUpperCase())}
                    </td>
                    <td
                      className="px-4 py-3 font-mono tabular-nums text-xs"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                      title={entry.resource_id}
                    >
                      {truncateId(entry.resource_id)}
                    </td>
                    <td
                      className="px-4 py-3 font-mono tabular-nums text-xs"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      {entry.actor_ip}
                    </td>
                  </tr>

                  {/* Expanded detail row */}
                  {isExpanded && (
                    <tr
                      style={{
                        borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                      }}
                    >
                      <td colSpan={7} className="px-6 py-4">
                        <div className="grid gap-4 sm:grid-cols-2">
                          {/* Old state */}
                          <div>
                            <span
                              className="mb-1 block text-xs font-heading font-medium"
                              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                            >
                              Previous State
                            </span>
                            <pre
                              className="overflow-x-auto rounded-lg p-3 font-mono text-xs"
                              style={{
                                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                                color: "var(--thittam-foreground, #0f172a)",
                                border: "1px solid var(--thittam-border, #e2e8f0)",
                                maxHeight: "200px",
                              }}
                            >
                              {entry.old_state
                                ? JSON.stringify(entry.old_state, null, 2)
                                : "null"}
                            </pre>
                          </div>

                          {/* New state */}
                          <div>
                            <span
                              className="mb-1 block text-xs font-heading font-medium"
                              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                            >
                              New State
                            </span>
                            <pre
                              className="overflow-x-auto rounded-lg p-3 font-mono text-xs"
                              style={{
                                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                                color: "var(--thittam-foreground, #0f172a)",
                                border: "1px solid var(--thittam-border, #e2e8f0)",
                                maxHeight: "200px",
                              }}
                            >
                              {entry.new_state
                                ? JSON.stringify(entry.new_state, null, 2)
                                : "null"}
                            </pre>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </tbody>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
