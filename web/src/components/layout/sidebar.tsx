"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  FolderKanban,
  Wallet,
  Receipt,
  Package,
  BarChart3,
  Users,
  Settings,
  ChevronLeft,
  ChevronRight,
  Clapperboard,
  Building2,
  CalendarDays,
  Server,
  Warehouse,
  HardHat,
  DollarSign,
  CreditCard,
  type LucideIcon,
} from "lucide-react";
import { useAuth } from "@/lib/auth/context";
import { useTheme } from "@/lib/themes/provider";

// Map icon name strings from vertical definitions to actual components
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
  Warehouse,
  HardHat,
  DollarSign,
  CreditCard,
};

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

interface MenuItem {
  key: string;
  label: string;
  href: string;
  iconName: string;
  fallbackIcon: LucideIcon;
  roles?: string[];
}

interface MenuSection {
  title: string;
  items: MenuItem[];
}

function useSidebarMenu(): MenuSection[] {
  const { entityLabels, theme } = useTheme();
  const sidebarIcons = theme.icons.sidebar;

  return [
    {
      title: "Main",
      items: [
        {
          key: "dashboard",
          label: "Dashboard",
          href: "/",
          iconName: sidebarIcons.dashboard ?? "LayoutDashboard",
          fallbackIcon: LayoutDashboard,
        },
        {
          key: "projects",
          label: entityLabels.projectPlural,
          href: "/projects",
          iconName: sidebarIcons.productions ?? "FolderKanban",
          fallbackIcon: FolderKanban,
        },
        {
          key: "budgets",
          label: "Budgets",
          href: "/budgets",
          iconName: sidebarIcons.budgets ?? "DollarSign",
          fallbackIcon: Wallet,
        },
        {
          key: "expenses",
          label: "Expenses",
          href: "/expenses",
          iconName: sidebarIcons.expenses ?? "Receipt",
          fallbackIcon: Receipt,
        },
        {
          key: "inventory",
          label: "Inventory",
          href: "/inventory",
          iconName: sidebarIcons.inventory ?? "Package",
          fallbackIcon: Package,
        },
      ],
    },
    {
      title: "Analytics",
      items: [
        {
          key: "reports",
          label: "Reports",
          href: "/reports",
          iconName: sidebarIcons.reports ?? "BarChart3",
          fallbackIcon: BarChart3,
        },
        {
          key: "team",
          label: entityLabels.teamMemberPlural,
          href: "/team",
          iconName: sidebarIcons.team ?? "Users",
          fallbackIcon: Users,
        },
      ],
    },
    {
      title: "Admin",
      items: [
        {
          key: "billing",
          label: "Billing",
          href: "/billing",
          iconName: "CreditCard",
          fallbackIcon: CreditCard,
          roles: ["admin", "owner"],
        },
        {
          key: "settings",
          label: "Settings",
          href: "/settings",
          iconName: sidebarIcons.settings ?? "Settings",
          fallbackIcon: Settings,
          roles: ["admin", "owner"],
        },
      ],
    },
  ];
}

function resolveIcon(iconName: string, fallback: LucideIcon): LucideIcon {
  return iconMap[iconName] ?? fallback;
}

export function Sidebar({ collapsed, onToggle }: SidebarProps) {
  const pathname = usePathname();
  const { user, tenant } = useAuth();
  const menu = useSidebarMenu();

  function isActive(href: string) {
    if (href === "/") return pathname === "/";
    return pathname.startsWith(href);
  }

  function canSee(item: MenuItem) {
    if (!item.roles) return true;
    return user?.roles.some((r) => item.roles!.includes(r)) ?? false;
  }

  return (
    <aside
      className={`flex h-full flex-col transition-[width] duration-200 ${
        collapsed ? "w-16" : "w-60"
      }`}
      style={{
        backgroundColor: "var(--thittam-sidebar-bg, #f8fafc)",
        color: "var(--thittam-sidebar-text, #64748b)",
        borderRight: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      {/* Brand */}
      <div
        className="flex h-14 items-center justify-between px-3"
        style={{
          borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        {!collapsed && (
          <div className="flex items-center gap-2">
            <div
              className="flex h-7 w-7 items-center justify-center rounded-md text-xs font-bold"
              style={{
                backgroundColor: "var(--thittam-primary, #3b82f6)",
                color: "var(--thittam-primary-foreground, #fff)",
              }}
            >
              T
            </div>
            <span
              className="text-sm font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Thittam
            </span>
          </div>
        )}
        <button
          onClick={onToggle}
          className="flex h-7 w-7 items-center justify-center rounded-md opacity-70 hover:opacity-100"
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {collapsed ? (
            <ChevronRight className="h-4 w-4" />
          ) : (
            <ChevronLeft className="h-4 w-4" />
          )}
        </button>
      </div>

      {/* Menu */}
      <nav className="flex-1 overflow-y-auto px-2 py-3">
        {menu.map((section) => {
          const visibleItems = section.items.filter(canSee);
          if (visibleItems.length === 0) return null;

          return (
            <div key={section.title} className="mb-4">
              {!collapsed && (
                <p className="mb-1 px-2 text-[11px] font-medium uppercase tracking-wider opacity-60">
                  {section.title}
                </p>
              )}
              <ul className="flex flex-col gap-0.5">
                {visibleItems.map((item) => {
                  const active = isActive(item.href);
                  const Icon = resolveIcon(item.iconName, item.fallbackIcon);

                  return (
                    <li key={item.key}>
                      <Link
                        href={item.href}
                        title={collapsed ? item.label : undefined}
                        className={`flex items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors ${
                          collapsed ? "justify-center" : ""
                        }`}
                        style={
                          active
                            ? {
                                backgroundColor:
                                  "var(--thittam-sidebar-active-bg, #e0e7ff)",
                                color:
                                  "var(--thittam-sidebar-active-item, #3b82f6)",
                                fontWeight: 500,
                              }
                            : undefined
                        }
                      >
                        <Icon className="h-5 w-5 shrink-0" />
                        {!collapsed && <span>{item.label}</span>}
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          );
        })}
      </nav>

      {/* Tenant info */}
      {tenant && !collapsed && (
        <div
          className="px-4 py-3"
          style={{
            borderTop: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <p
            className="truncate text-xs font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {tenant.name}
          </p>
          <span
            className="mt-0.5 inline-block rounded-full px-2 py-0.5 text-[10px] font-medium"
            style={{
              backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 15%, transparent)",
              color: "var(--thittam-primary, #3b82f6)",
            }}
          >
            {tenant.plan}
          </span>
        </div>
      )}
    </aside>
  );
}
