"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type NotificationCategory = "expense" | "budget" | "production" | "system";

export interface Notification {
  id: string;
  type: string; // e.g. "expense.submitted"
  category: NotificationCategory;
  title: string;
  body: string;
  read: boolean;
  created_at: string;
  link?: string; // optional deep-link into the app
}

interface NotificationContextValue {
  notifications: Notification[];
  unreadCount: number;
  markRead: (id: string) => void;
  markAllRead: () => void;
}

// ---------------------------------------------------------------------------
// Mock data — 10 notifications for XYZ CBA Productions
// ---------------------------------------------------------------------------

const INITIAL_NOTIFICATIONS: Notification[] = [
  {
    id: "notif-001",
    type: "expense.submitted",
    category: "expense",
    title: "New expense submitted",
    body: 'Arjun Mehta submitted "Drone Operator — Aerial Shots Package" for ₹1,80,000 — awaiting approval.',
    read: false,
    created_at: "2026-04-10T09:15:00Z",
    link: "/expenses/exp-008",
  },
  {
    id: "notif-002",
    type: "system.plan_limit_warning",
    category: "system",
    title: "Storage approaching limit",
    body: "You've used 24.8% of your Professional plan storage (12.4 GB / 50 GB). No action needed yet.",
    read: false,
    created_at: "2026-04-10T03:00:00Z",
    link: "/billing",
  },
  {
    id: "notif-003",
    type: "budget.approved",
    category: "budget",
    title: "Budget approved",
    body: "Priya Sharma approved budget version 3 for The Last Horizon (₹1,50,00,000 total).",
    read: false,
    created_at: "2026-04-09T17:30:00Z",
    link: "/budgets",
  },
  {
    id: "notif-004",
    type: "production.phase_changed",
    category: "production",
    title: "Production phase changed",
    body: "The Last Horizon moved from Pre-production to Production. Principal photography begins today.",
    read: false,
    created_at: "2026-04-09T10:00:00Z",
    link: "/productions/prod-001",
  },
  {
    id: "notif-005",
    type: "expense.approved",
    category: "expense",
    title: "Expense approved",
    body: 'Deepa Nair approved "Ladakh Location Recce & Permits" (₹7,50,000).',
    read: true,
    created_at: "2026-04-08T16:00:00Z",
    link: "/expenses/exp-005",
  },
  {
    id: "notif-006",
    type: "expense.rejected",
    category: "expense",
    title: "Expense rejected",
    body: "IMAX Camera Rental (₹35,00,000) was rejected — exceeds the approved budget ceiling for equipment.",
    read: true,
    created_at: "2026-03-30T09:20:00Z",
    link: "/expenses/exp-010",
  },
  {
    id: "notif-007",
    type: "budget.locked",
    category: "budget",
    title: "Budget locked",
    body: "Priya Sharma locked budget version 2 for The Last Horizon. No further edits are permitted.",
    read: true,
    created_at: "2026-03-28T14:00:00Z",
    link: "/budgets",
  },
  {
    id: "notif-008",
    type: "expense.submitted",
    category: "expense",
    title: "New expense submitted",
    body: 'Priya Sharma submitted "Heritage Building Shoot Location" for ₹2,50,000 — pending approval.',
    read: true,
    created_at: "2026-03-12T11:30:00Z",
    link: "/expenses/exp-007",
  },
  {
    id: "notif-009",
    type: "expense.approved",
    category: "expense",
    title: "Expense paid",
    body: "Film City Studio Rental (₹18,00,000) has been marked as paid. PO-001 closed.",
    read: true,
    created_at: "2026-02-20T08:00:00Z",
    link: "/expenses/exp-001",
  },
  {
    id: "notif-010",
    type: "system.trial_ending",
    category: "system",
    title: "Trial period ending",
    body: "Your 14-day trial ended. You have been upgraded to Professional. Welcome aboard!",
    read: true,
    created_at: "2026-01-12T09:00:00Z",
    link: "/billing",
  },
];

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const NotificationContext = createContext<NotificationContextValue | null>(null);

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [notifications, setNotifications] = useState<Notification[]>(INITIAL_NOTIFICATIONS);

  const unreadCount = useMemo(
    () => notifications.filter((n) => !n.read).length,
    [notifications],
  );

  const markRead = useCallback((id: string) => {
    setNotifications((prev) =>
      prev.map((n) => (n.id === id ? { ...n, read: true } : n)),
    );
  }, []);

  const markAllRead = useCallback(() => {
    setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
  }, []);

  const value = useMemo<NotificationContextValue>(
    () => ({ notifications, unreadCount, markRead, markAllRead }),
    [notifications, unreadCount, markRead, markAllRead],
  );

  return (
    <NotificationContext.Provider value={value}>
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotifications(): NotificationContextValue {
  const ctx = useContext(NotificationContext);
  if (!ctx) {
    throw new Error("useNotifications must be used within a <NotificationProvider>");
  }
  return ctx;
}
