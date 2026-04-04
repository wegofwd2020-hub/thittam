"use client";

import {
  FolderKanban,
  Inbox,
  Search,
  FileText,
  Package,
  Users,
  BarChart3,
  type LucideIcon,
} from "lucide-react";

// ---------------------------------------------------------------------------
// EmptyState — centered placeholder for empty lists / search results.
// ---------------------------------------------------------------------------

interface EmptyStateProps {
  icon?: string;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
}

const iconMap: Record<string, LucideIcon> = {
  FolderKanban,
  Inbox,
  Search,
  FileText,
  Package,
  Users,
  BarChart3,
};

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  const Icon = icon ? iconMap[icon] ?? Inbox : Inbox;

  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div
        className="mb-4 flex h-14 w-14 items-center justify-center rounded-full"
        style={{
          backgroundColor: "var(--thittam-muted, #f1f5f9)",
          color: "var(--thittam-muted-foreground, #94a3b8)",
        }}
      >
        <Icon className="h-6 w-6" />
      </div>
      <h3
        className="text-base font-semibold font-heading"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        {title}
      </h3>
      {description && (
        <p
          className="mt-1 max-w-sm text-sm font-body"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {description}
        </p>
      )}
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="mt-4 rounded-lg px-4 py-2 text-sm font-medium transition-colors hover:opacity-90"
          style={{
            backgroundColor: "var(--thittam-primary, #3b82f6)",
            color: "var(--thittam-primary-foreground, #fff)",
          }}
        >
          {action.label}
        </button>
      )}
    </div>
  );
}
