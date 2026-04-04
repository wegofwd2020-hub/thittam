import {
  LayoutDashboard,
  FolderKanban,
  Wallet,
  Receipt,
  Package,
  BarChart3,
  Users,
  Settings,
  Clapperboard,
  Building2,
  CalendarDays,
  Server,
  HardHat,
  DollarSign,
  FileText,
  type LucideIcon,
} from "lucide-react";

// ---------------------------------------------------------------------------
// PageHeader — page title with optional description, icon, and action slot.
// ---------------------------------------------------------------------------

interface PageHeaderProps {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  icon?: string; // lucide icon name
}

const iconMap: Record<string, LucideIcon> = {
  LayoutDashboard,
  FolderKanban,
  Wallet,
  Receipt,
  Package,
  BarChart3,
  Users,
  Settings,
  Clapperboard,
  Building2,
  CalendarDays,
  Server,
  HardHat,
  DollarSign,
  FileText,
};

export function PageHeader({ title, description, actions, icon }: PageHeaderProps) {
  const Icon = icon ? iconMap[icon] ?? null : null;

  return (
    <div className="flex items-start justify-between gap-4 pb-6">
      <div className="flex items-center gap-3">
        {Icon && (
          <div
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
            style={{
              backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 12%, transparent)",
              color: "var(--thittam-primary, #3b82f6)",
            }}
          >
            <Icon className="h-5 w-5" />
          </div>
        )}
        <div>
          <h1
            className="text-2xl font-bold tracking-tight font-heading"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {title}
          </h1>
          {description && (
            <p
              className="mt-1 text-sm font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              {description}
            </p>
          )}
        </div>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}
