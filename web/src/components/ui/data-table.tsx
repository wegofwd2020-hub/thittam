"use client";

import { useCallback, useMemo, useState } from "react";
import { TableSkeleton } from "./loading-skeleton";

// ---------------------------------------------------------------------------
// DataTable — generic, sortable data table with loading & empty states.
// ---------------------------------------------------------------------------

export interface Column<T> {
  key: string;
  header: string;
  render?: (row: T) => React.ReactNode;
  type?: "text" | "number" | "date" | "status";
  sortable?: boolean;
}

export interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  onRowClick?: (row: T) => void;
  isLoading?: boolean;
  emptyMessage?: string;
}

type SortDir = "asc" | "desc";

export function DataTable<T extends Record<string, unknown>>({
  columns,
  data,
  onRowClick,
  isLoading = false,
  emptyMessage = "No data to display.",
}: DataTableProps<T>) {
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const handleSort = useCallback(
    (key: string) => {
      if (sortKey === key) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      } else {
        setSortKey(key);
        setSortDir("asc");
      }
    },
    [sortKey],
  );

  const sortedData = useMemo(() => {
    if (!sortKey) return data;
    return [...data].sort((a, b) => {
      const aVal = a[sortKey];
      const bVal = b[sortKey];
      if (aVal == null && bVal == null) return 0;
      if (aVal == null) return 1;
      if (bVal == null) return -1;
      if (typeof aVal === "number" && typeof bVal === "number") {
        return sortDir === "asc" ? aVal - bVal : bVal - aVal;
      }
      const cmp = String(aVal).localeCompare(String(bVal));
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [data, sortKey, sortDir]);

  if (isLoading) {
    return <TableSkeleton rows={5} cols={columns.length} />;
  }

  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center rounded-lg border py-16 font-body"
        style={{
          borderColor: "var(--thittam-border, #e2e8f0)",
          color: "var(--thittam-muted-foreground, #64748b)",
        }}
      >
        {emptyMessage}
      </div>
    );
  }

  function cellFont(type?: string) {
    if (type === "number" || type === "date") return "font-mono tabular-nums";
    return "font-body";
  }

  return (
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
            {columns.map((col) => (
              <th
                key={col.key}
                className={`px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider font-heading ${
                  col.sortable ? "cursor-pointer select-none hover:opacity-80" : ""
                }`}
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                onClick={col.sortable ? () => handleSort(col.key) : undefined}
              >
                <span className="inline-flex items-center gap-1">
                  {col.header}
                  {col.sortable && sortKey === col.key && (
                    <span aria-hidden="true">
                      {sortDir === "asc" ? "\u2191" : "\u2193"}
                    </span>
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sortedData.map((row, idx) => (
            <tr
              key={idx}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              className={`transition-colors ${
                idx % 2 === 1 ? "bg-gray-50/50" : ""
              } ${onRowClick ? "cursor-pointer hover:bg-gray-100/70" : ""}`}
              style={{
                borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  className={`px-4 py-3 ${cellFont(col.type)}`}
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {col.render
                    ? col.render(row)
                    : (row[col.key] as React.ReactNode) ?? "\u2014"}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
