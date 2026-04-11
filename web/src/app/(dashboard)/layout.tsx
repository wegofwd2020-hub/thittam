"use client";

import { useState } from "react";
import { AuthProvider, useAuth } from "@/lib/auth/context";
import { ProtectedRoute } from "@/lib/auth/protected-route";
import { ThemeProvider } from "@/lib/themes/provider";
import { NotificationProvider } from "@/lib/notifications/context";
import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";

function DashboardShell({ children }: { children: React.ReactNode }) {
  const { verticalId } = useAuth();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  return (
    <ThemeProvider verticalId={verticalId ?? "movie-production"}>
    <NotificationProvider>
      <div className="flex h-screen overflow-hidden bg-[var(--thittam-background,#fff)]">
        {/* Mobile overlay */}
        {mobileMenuOpen && (
          <div
            className="fixed inset-0 z-40 bg-black/50 lg:hidden"
            onClick={() => setMobileMenuOpen(false)}
          />
        )}

        {/* Sidebar - desktop */}
        <div className="hidden lg:flex">
          <Sidebar
            collapsed={sidebarCollapsed}
            onToggle={() => setSidebarCollapsed((prev) => !prev)}
          />
        </div>

        {/* Sidebar - mobile */}
        <div
          className={`fixed inset-y-0 left-0 z-50 lg:hidden transition-transform duration-200 ${
            mobileMenuOpen ? "translate-x-0" : "-translate-x-full"
          }`}
        >
          <Sidebar
            collapsed={false}
            onToggle={() => setMobileMenuOpen(false)}
          />
        </div>

        {/* Main content area */}
        <div className="flex flex-1 flex-col overflow-hidden">
          <Topbar onMenuToggle={() => setMobileMenuOpen((prev) => !prev)} />
          <main className="flex-1 overflow-y-auto p-6">{children}</main>
        </div>
      </div>
    </NotificationProvider>
    </ThemeProvider>
  );
}

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <AuthProvider>
      <ProtectedRoute>
        <DashboardShell>{children}</DashboardShell>
      </ProtectedRoute>
    </AuthProvider>
  );
}
