"use client";

import { useState, useRef, useEffect } from "react";
import Link from "next/link";
import { Menu, ChevronRight, User, Settings, LogOut } from "lucide-react";
import { useAuth } from "@/lib/auth/context";
import { NotificationBell } from "@/components/layout/notification-bell";

interface TopbarProps {
  onMenuToggle: () => void;
}

export function Topbar({ onMenuToggle }: TopbarProps) {
  const { user, logout } = useAuth();
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node)
      ) {
        setDropdownOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const initials = user?.displayName
    ? user.displayName
        .split(" ")
        .map((n) => n[0])
        .join("")
        .slice(0, 2)
        .toUpperCase()
    : "?";

  return (
    <header
      className="flex h-14 shrink-0 items-center justify-between px-4"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      {/* Left side */}
      <div className="flex items-center gap-3">
        <button
          onClick={onMenuToggle}
          className="flex h-8 w-8 items-center justify-center rounded-md hover:opacity-80 lg:hidden"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          aria-label="Toggle menu"
        >
          <Menu className="h-5 w-5" />
        </button>
        {/* Breadcrumb placeholder */}
        <nav className="hidden items-center gap-1 text-sm sm:flex">
          <span
            className="font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Dashboard
          </span>
          <ChevronRight
            className="h-3 w-3"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          />
        </nav>
      </div>

      {/* Right side */}
      <div className="flex items-center gap-2">
        {/* Notification bell */}
        <NotificationBell />

        {/* User dropdown */}
        <div className="relative" ref={dropdownRef}>
          <button
            onClick={() => setDropdownOpen((prev) => !prev)}
            className="flex h-8 w-8 items-center justify-center rounded-full text-xs font-medium"
            style={{
              backgroundColor: "var(--thittam-primary, #3b82f6)",
              color: "var(--thittam-primary-foreground, #fff)",
            }}
            aria-label="User menu"
          >
            {initials}
          </button>

          {dropdownOpen && (
            <div
              className="absolute right-0 z-50 mt-2 w-56 rounded-md py-1 shadow-lg"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              {user && (
                <div
                  className="px-4 py-2"
                  style={{
                    borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                  }}
                >
                  <p
                    className="truncate text-sm font-medium"
                    style={{ color: "var(--thittam-foreground, #0f172a)" }}
                  >
                    {user.displayName}
                  </p>
                  <p
                    className="truncate text-xs"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {user.email}
                  </p>
                </div>
              )}
              <Link
                href="/profile"
                onClick={() => setDropdownOpen(false)}
                className="flex items-center gap-2 px-4 py-2 text-sm hover:opacity-80"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                <User className="h-4 w-4" />
                Profile
              </Link>
              <Link
                href="/settings"
                onClick={() => setDropdownOpen(false)}
                className="flex items-center gap-2 px-4 py-2 text-sm hover:opacity-80"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                <Settings className="h-4 w-4" />
                Settings
              </Link>
              <div
                style={{
                  borderTop: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              />
              <button
                onClick={() => {
                  setDropdownOpen(false);
                  logout();
                }}
                className="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:opacity-80 dark:text-red-400"
              >
                <LogOut className="h-4 w-4" />
                Sign out
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
