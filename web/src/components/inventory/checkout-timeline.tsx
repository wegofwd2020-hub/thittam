"use client";

import { DataTable, type Column } from "@/components/ui/data-table";

// ---------------------------------------------------------------------------
// CheckoutTimeline — table of checkout history for an asset.
// ---------------------------------------------------------------------------

export interface CheckoutEntry {
  id: string;
  production: string;
  checkedOutTo: string;
  checkedOutAt: string;
  checkedInAt: string | null;
  conditionOut: string;
  conditionIn: string | null;
}

interface CheckoutTimelineProps {
  entries: CheckoutEntry[];
}

function ConditionBadge({ condition }: { condition: string }) {
  const colorMap: Record<string, { bg: string; text: string }> = {
    good: { bg: "bg-green-100", text: "text-green-700" },
    fair: { bg: "bg-amber-100", text: "text-amber-700" },
    damaged: { bg: "bg-red-100", text: "text-red-700" },
  };
  const style = colorMap[condition] ?? { bg: "bg-gray-100", text: "text-gray-600" };

  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium font-heading capitalize ${style.bg} ${style.text}`}
    >
      {condition}
    </span>
  );
}

const columns: Column<CheckoutEntry>[] = [
  {
    key: "production",
    header: "Production",
    render: (row) => (
      <span
        className="font-heading text-sm font-medium"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        {row.production}
      </span>
    ),
  },
  {
    key: "checkedOutTo",
    header: "Checked Out To",
    render: (row) => (
      <span
        className="font-body text-sm"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        {row.checkedOutTo}
      </span>
    ),
  },
  {
    key: "checkedOutAt",
    header: "Out Date",
    type: "date",
    render: (row) => (
      <span
        className="font-mono tabular-nums text-sm"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        {row.checkedOutAt}
      </span>
    ),
  },
  {
    key: "checkedInAt",
    header: "Return Date",
    type: "date",
    render: (row) => (
      <span
        className="font-mono tabular-nums text-sm"
        style={{ color: row.checkedInAt ? "var(--thittam-foreground, #0f172a)" : "var(--thittam-muted-foreground, #64748b)" }}
      >
        {row.checkedInAt ?? "\u2014"}
      </span>
    ),
  },
  {
    key: "conditionOut",
    header: "Condition Out",
    render: (row) => <ConditionBadge condition={row.conditionOut} />,
  },
  {
    key: "conditionIn",
    header: "Condition In",
    render: (row) =>
      row.conditionIn ? (
        <ConditionBadge condition={row.conditionIn} />
      ) : (
        <span
          className="font-mono text-xs"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {"\u2014"}
        </span>
      ),
  },
];

export function CheckoutTimeline({ entries }: CheckoutTimelineProps) {
  return (
    <DataTable<Record<string, unknown>>
      columns={columns as unknown as Column<Record<string, unknown>>[]}
      data={entries as unknown as Record<string, unknown>[]}
      emptyMessage="No checkout history"
    />
  );
}
