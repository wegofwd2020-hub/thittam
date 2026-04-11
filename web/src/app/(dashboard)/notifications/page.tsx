"use client";

import { useState, useMemo } from "react";
import {
  Receipt,
  Wallet,
  FolderKanban,
  AlertTriangle,
  Clock,
  CheckCircle2,
  XCircle,
  Lock,
  Bell,
} from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import {
  useNotifications,
  type Notification,
  type NotificationCategory,
} from "@/lib/notifications/context";

// ---------------------------------------------------------------------------
// Icon resolver (mirrors notification-bell.tsx)
// ---------------------------------------------------------------------------

interface TypeStyle {
  Icon: React.ElementType;
  bg: string;
  color: string;
}

const TYPE_STYLES: Record<string, TypeStyle> = {
  "expense.submitted":         { Icon: Receipt,       bg: "#dbeafe", color: "#1d4ed8" },
  "expense.approved":          { Icon: CheckCircle2,  bg: "#d1fae5", color: "#065f46" },
  "expense.rejected":          { Icon: XCircle,       bg: "#fee2e2", color: "#991b1b" },
  "budget.approved":           { Icon: Wallet,        bg: "#d1fae5", color: "#065f46" },
  "budget.locked":             { Icon: Lock,          bg: "#ede9fe", color: "#6b21a8" },
  "production.phase_changed":  { Icon: FolderKanban,  bg: "#dbeafe", color: "#1d4ed8" },
  "system.plan_limit_warning": { Icon: AlertTriangle, bg: "#fef3c7", color: "#92400e" },
  "system.trial_ending":       { Icon: Clock,         bg: "#fef3c7", color: "#92400e" },
};

const FALLBACK_STYLE: TypeStyle = { Icon: Bell, bg: "#f1f5f9", color: "#64748b" };

function typeStyle(type: string): TypeStyle {
  return TYPE_STYLES[type] ?? FALLBACK_STYLE;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(iso).toLocaleDateString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

// ---------------------------------------------------------------------------
// Filter definitions
// ---------------------------------------------------------------------------

const FILTER_OPTIONS: { key: "all" | "unread" | NotificationCategory; label: string }[] = [
  { key: "all",        label: "All" },
  { key: "unread",     label: "Unread" },
  { key: "expense",    label: "Expense" },
  { key: "budget",     label: "Budget" },
  { key: "production", label: "Production" },
  { key: "system",     label: "System" },
];

type FilterKey = (typeof FILTER_OPTIONS)[number]["key"];

// ---------------------------------------------------------------------------
// Notification row
// ---------------------------------------------------------------------------

function NotificationRow({
  notif,
  onRead,
}: {
  notif: Notification;
  onRead: () => void;
}) {
  const { Icon, bg, color } = typeStyle(notif.type);

  return (
    <button
      onClick={onRead}
      className="flex w-full items-start gap-4 rounded-xl px-5 py-4 text-left transition-colors hover:opacity-90"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        border: notif.read
          ? "1px solid var(--thittam-border, #e2e8f0)"
          : "1.5px solid var(--thittam-primary, #3b82f6)",
        borderLeft: notif.read
          ? "1px solid var(--thittam-border, #e2e8f0)"
          : "4px solid var(--thittam-primary, #3b82f6)",
      }}
    >
      {/* Type icon */}
      <span
        className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full"
        style={{ backgroundColor: bg }}
      >
        <Icon className="h-4 w-4" style={{ color }} />
      </span>

      {/* Body */}
      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-2">
          <p
            className={`text-sm ${notif.read ? "font-body" : "font-heading font-semibold"}`}
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {notif.title}
          </p>
          <span
            className="shrink-0 font-mono tabular-nums text-xs"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {relativeTime(notif.created_at)}
          </span>
        </div>
        <p
          className="mt-1 text-sm font-body leading-relaxed"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {notif.body}
        </p>
      </div>

      {/* Unread dot */}
      {!notif.read && (
        <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-blue-500" />
      )}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function NotificationsPage() {
  const { notifications, unreadCount, markRead, markAllRead } = useNotifications();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");

  const filtered = useMemo(() => {
    return notifications.filter((n) => {
      if (activeFilter === "all") return true;
      if (activeFilter === "unread") return !n.read;
      return n.category === activeFilter;
    });
  }, [notifications, activeFilter]);

  return (
    <div className="mx-auto max-w-3xl">
      <PageHeader
        title="Notifications"
        description="Activity across your productions, budgets, and expenses"
        icon="Bell"
        actions={
          unreadCount > 0 ? (
            <button
              onClick={markAllRead}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-opacity hover:opacity-80"
              style={{
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
                backgroundColor: "var(--thittam-background, #fff)",
              }}
            >
              Mark all read
            </button>
          ) : null
        }
      />

      {/* Filter chips */}
      <div className="mb-6 flex flex-wrap gap-2">
        {FILTER_OPTIONS.map((opt) => {
          const isActive = activeFilter === opt.key;
          const showCount = opt.key === "unread" && unreadCount > 0;
          return (
            <button
              key={opt.key}
              onClick={() => setActiveFilter(opt.key)}
              className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-heading font-medium transition-colors ${
                isActive ? "text-white" : ""
              }`}
              style={
                isActive
                  ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
                  : {
                      backgroundColor: "var(--thittam-muted, #f1f5f9)",
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }
              }
            >
              {opt.label}
              {showCount && (
                <span
                  className="rounded-full px-1 text-[10px] font-bold"
                  style={{
                    backgroundColor: isActive ? "rgba(255,255,255,0.25)" : "#3b82f6",
                    color: isActive ? "#fff" : "#fff",
                  }}
                >
                  {unreadCount}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Notification list */}
      {filtered.length === 0 ? (
        <div
          className="flex flex-col items-center justify-center rounded-xl border py-16"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <CheckCircle2 className="h-10 w-10 text-emerald-400" />
          <p
            className="mt-3 font-heading text-base font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {activeFilter === "unread" ? "All caught up!" : "No notifications here"}
          </p>
          <p
            className="mt-1 text-sm font-body"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {activeFilter === "unread"
              ? "You have no unread notifications."
              : "Nothing to show for this filter."}
          </p>
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {filtered.map((notif) => (
            <NotificationRow
              key={notif.id}
              notif={notif}
              onRead={() => markRead(notif.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
