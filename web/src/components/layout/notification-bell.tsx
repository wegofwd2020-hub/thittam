"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import {
  Bell,
  Receipt,
  Wallet,
  FolderKanban,
  AlertTriangle,
  Clock,
  CheckCircle2,
  XCircle,
  Lock,
} from "lucide-react";
import { useNotifications, type Notification } from "@/lib/notifications/context";

// ---------------------------------------------------------------------------
// Icon resolver — maps notification type to a lucide icon + colour
// ---------------------------------------------------------------------------

interface TypeStyle {
  Icon: React.ElementType;
  bg: string;
  color: string;
}

const TYPE_STYLES: Record<string, TypeStyle> = {
  "expense.submitted":          { Icon: Receipt,       bg: "#dbeafe", color: "#1d4ed8" },
  "expense.approved":           { Icon: CheckCircle2,  bg: "#d1fae5", color: "#065f46" },
  "expense.rejected":           { Icon: XCircle,       bg: "#fee2e2", color: "#991b1b" },
  "budget.approved":            { Icon: Wallet,        bg: "#d1fae5", color: "#065f46" },
  "budget.locked":              { Icon: Lock,          bg: "#ede9fe", color: "#6b21a8" },
  "production.phase_changed":   { Icon: FolderKanban,  bg: "#dbeafe", color: "#1d4ed8" },
  "system.plan_limit_warning":  { Icon: AlertTriangle, bg: "#fef3c7", color: "#92400e" },
  "system.trial_ending":        { Icon: Clock,         bg: "#fef3c7", color: "#92400e" },
};

const FALLBACK_STYLE: TypeStyle = { Icon: Bell, bg: "#f1f5f9", color: "#64748b" };

function notifTypeStyle(type: string): TypeStyle {
  return TYPE_STYLES[type] ?? FALLBACK_STYLE;
}

// ---------------------------------------------------------------------------
// Relative time helper
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
  return new Date(iso).toLocaleDateString("en-IN", { month: "short", day: "2-digit" });
}

// ---------------------------------------------------------------------------
// Single notification row inside the dropdown
// ---------------------------------------------------------------------------

function DropdownNotifRow({
  notif,
  onRead,
}: {
  notif: Notification;
  onRead: () => void;
}) {
  const { Icon, bg, color } = notifTypeStyle(notif.type);

  return (
    <button
      onClick={onRead}
      className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-gray-50/80"
    >
      {/* Type icon */}
      <span
        className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full"
        style={{ backgroundColor: bg }}
      >
        <Icon className="h-3.5 w-3.5" style={{ color }} />
      </span>

      {/* Text */}
      <div className="min-w-0 flex-1">
        <p
          className={`truncate text-xs ${notif.read ? "font-body" : "font-heading font-semibold"}`}
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {notif.title}
        </p>
        <p
          className="mt-0.5 line-clamp-2 text-xs font-body leading-relaxed"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {notif.body}
        </p>
        <p
          className="mt-1 font-mono tabular-nums text-[10px]"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {relativeTime(notif.created_at)}
        </p>
      </div>

      {/* Unread dot */}
      {!notif.read && (
        <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-blue-500" />
      )}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Bell button with badge + dropdown
// ---------------------------------------------------------------------------

export function NotificationBell() {
  const { notifications, unreadCount, markRead, markAllRead } = useNotifications();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    function handler(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  // 5 most recent unread for the dropdown
  const recentUnread = notifications
    .filter((n) => !n.read)
    .slice(0, 5);

  return (
    <div className="relative" ref={containerRef}>
      {/* Bell button */}
      <button
        onClick={() => setOpen((prev) => !prev)}
        className="relative flex h-8 w-8 items-center justify-center rounded-md hover:opacity-80"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ""}`}
      >
        <Bell className="h-5 w-5" />
        {unreadCount > 0 && (
          <span className="absolute right-0.5 top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-blue-500 text-[9px] font-bold text-white leading-none">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </button>

      {/* Dropdown */}
      {open && (
        <div
          className="absolute right-0 z-50 mt-2 w-80 overflow-hidden rounded-xl shadow-lg"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          {/* Header */}
          <div
            className="flex items-center justify-between px-4 py-3"
            style={{ borderBottom: "1px solid var(--thittam-border, #e2e8f0)" }}
          >
            <span
              className="text-sm font-heading font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Notifications
              {unreadCount > 0 && (
                <span className="ml-1.5 inline-flex items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700">
                  {unreadCount} new
                </span>
              )}
            </span>
            {unreadCount > 0 && (
              <button
                onClick={markAllRead}
                className="text-xs font-heading font-medium transition-opacity hover:opacity-70"
                style={{ color: "var(--thittam-primary, #3b82f6)" }}
              >
                Mark all read
              </button>
            )}
          </div>

          {/* Notification list */}
          {recentUnread.length === 0 ? (
            <div className="px-4 py-8 text-center">
              <CheckCircle2 className="mx-auto h-8 w-8 text-emerald-400" />
              <p
                className="mt-2 text-sm font-body"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                All caught up!
              </p>
            </div>
          ) : (
            <div>
              {recentUnread.map((notif) => (
                <DropdownNotifRow
                  key={notif.id}
                  notif={notif}
                  onRead={() => {
                    markRead(notif.id);
                    setOpen(false);
                  }}
                />
              ))}
            </div>
          )}

          {/* Footer */}
          <div
            className="px-4 py-2.5"
            style={{ borderTop: "1px solid var(--thittam-border, #e2e8f0)" }}
          >
            <Link
              href="/notifications"
              onClick={() => setOpen(false)}
              className="block text-center text-xs font-heading font-medium transition-opacity hover:opacity-70"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            >
              View all notifications →
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}
