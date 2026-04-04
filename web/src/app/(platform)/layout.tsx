"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  MonitorPlay,
  Building2,
  Layers,
  Users,
  LogOut,
  ChevronLeft,
  ChevronRight,
  Shield,
  type LucideIcon,
} from "lucide-react";

// ---------------------------------------------------------------------------
// Platform Admin Layout
// Visually distinct from the tenant dashboard — dark slate sidebar,
// orange/red accent, no vertical theming.
// ---------------------------------------------------------------------------

interface PlatformMenuItem {
  label: string;
  href: string;
  icon: LucideIcon;
}

const MENU_ITEMS: PlatformMenuItem[] = [
  { label: "Dashboard", href: "/platform", icon: LayoutDashboard },
  { label: "Demo Provisioning", href: "/demo", icon: MonitorPlay },
  { label: "Tenants", href: "/tenants", icon: Building2 },
  { label: "Verticals", href: "/verticals", icon: Layers },
  { label: "Platform Users", href: "/platform-users", icon: Users },
];

function PlatformSidebar({
  collapsed,
  onToggle,
}: {
  collapsed: boolean;
  onToggle: () => void;
}) {
  const pathname = usePathname();

  function isActive(href: string) {
    if (href === "/platform") return pathname === "/platform";
    return pathname.startsWith(href);
  }

  return (
    <aside
      className={`flex h-full flex-col bg-slate-900 text-slate-300 transition-[width] duration-200 ${
        collapsed ? "w-16" : "w-60"
      }`}
    >
      {/* Brand */}
      <div className="flex h-14 items-center justify-between border-b border-slate-700 px-3">
        {!collapsed && (
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-orange-600 text-xs font-bold text-white">
              <Shield className="h-4 w-4" />
            </div>
            <span className="font-heading text-sm font-semibold text-white">
              Platform Admin
            </span>
          </div>
        )}
        <button
          onClick={onToggle}
          className="flex h-7 w-7 items-center justify-center rounded-md text-slate-400 hover:text-white"
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
        <ul className="flex flex-col gap-0.5">
          {MENU_ITEMS.map((item) => {
            const active = isActive(item.href);
            const Icon = item.icon;

            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  title={collapsed ? item.label : undefined}
                  className={`flex items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors ${
                    collapsed ? "justify-center" : ""
                  } ${
                    active
                      ? "bg-orange-600/20 font-medium text-orange-400"
                      : "text-slate-400 hover:bg-slate-800 hover:text-white"
                  }`}
                >
                  <Icon className="h-5 w-5 shrink-0" />
                  {!collapsed && <span>{item.label}</span>}
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>

      {/* Footer */}
      {!collapsed && (
        <div className="border-t border-slate-700 px-4 py-3">
          <p className="truncate font-heading text-xs font-medium text-white">
            Thittam Platform
          </p>
          <p className="mt-0.5 font-body text-[10px] text-slate-500">
            Super Admin Console
          </p>
        </div>
      )}
    </aside>
  );
}

function PlatformTopbar() {
  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4">
      {/* Left — breadcrumb area */}
      <div className="flex items-center gap-2">
        <span className="inline-flex items-center rounded-full bg-orange-100 px-2.5 py-0.5 font-heading text-xs font-medium text-orange-700">
          Platform Mode
        </span>
      </div>

      {/* Right */}
      <div className="flex items-center gap-3">
        <Link
          href="/"
          className="flex items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-1.5 font-heading text-xs font-medium text-slate-700 transition-colors hover:bg-slate-50"
        >
          <LogOut className="h-3.5 w-3.5" />
          Exit Platform
        </Link>
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-orange-600 text-xs font-medium text-white">
          SA
        </div>
      </div>
    </header>
  );
}

export default function PlatformLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  return (
    <div className="flex h-screen overflow-hidden bg-slate-50">
      <PlatformSidebar
        collapsed={sidebarCollapsed}
        onToggle={() => setSidebarCollapsed((prev) => !prev)}
      />
      <div className="flex flex-1 flex-col overflow-hidden">
        <PlatformTopbar />
        <main className="flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}
