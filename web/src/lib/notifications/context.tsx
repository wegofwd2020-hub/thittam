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

const INITIAL_NOTIFICATIONS: Notification[] = [];

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
