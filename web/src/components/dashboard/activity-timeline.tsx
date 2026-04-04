"use client";

// ---------------------------------------------------------------------------
// ActivityTimeline — vertical timeline of recent user actions.
// ---------------------------------------------------------------------------

export interface ActivityItem {
  id: string;
  actor: string;
  action: string;
  target: string;
  timestamp: string;
}

interface ActivityTimelineProps {
  items: ActivityItem[];
}

/** Returns a human-friendly relative time string from an ISO timestamp. */
function relativeTime(iso: string): string {
  const now = new Date();
  const then = new Date(iso);
  const diffMs = now.getTime() - then.getTime();
  const diffMin = Math.floor(diffMs / 60_000);

  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin} min ago`;

  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr} hour${diffHr === 1 ? "" : "s"} ago`;

  const diffDays = Math.floor(diffHr / 24);
  if (diffDays < 7) return `${diffDays} day${diffDays === 1 ? "" : "s"} ago`;

  return then.toLocaleDateString("en-IN", { day: "numeric", month: "short" });
}

export function ActivityTimeline({ items }: ActivityTimelineProps) {
  if (items.length === 0) {
    return (
      <p
        className="py-8 text-center text-sm font-body"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        No recent activity.
      </p>
    );
  }

  return (
    <ol className="relative ml-3 border-l-2 border-gray-200 space-y-6">
      {items.map((item) => (
        <li key={item.id} className="relative pl-6">
          {/* Dot on the timeline */}
          <span
            className="absolute -left-[7px] top-1 h-3 w-3 rounded-full border-2 border-white"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          />

          {/* Content */}
          <div>
            <p
              className="text-sm"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              <span className="font-heading font-semibold">{item.actor}</span>{" "}
              <span className="font-body">{item.action}</span>{" "}
              <span className="font-body font-medium">{item.target}</span>
            </p>
            <time
              className="mt-0.5 block text-xs font-mono tabular-nums"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              dateTime={item.timestamp}
            >
              {relativeTime(item.timestamp)}
            </time>
          </div>
        </li>
      ))}
    </ol>
  );
}
