"use client";

import { useAuth } from "@/lib/auth/context";
import { useTheme } from "@/lib/themes/provider";
import {
  FolderKanban,
  Wallet,
  Receipt,
  Package,
  BarChart3,
  Users,
} from "lucide-react";
import Link from "next/link";

interface QuickCard {
  label: string;
  href: string;
  description: string;
  icon: React.ReactNode;
}

export default function DashboardPage() {
  const { user } = useAuth();
  const { entityLabels } = useTheme();

  const quickCards: QuickCard[] = [
    {
      label: entityLabels.projectPlural,
      href: "/projects",
      description: `Manage your active ${entityLabels.projectPlural.toLowerCase()} and ${entityLabels.phasePlural.toLowerCase()}.`,
      icon: <FolderKanban className="h-6 w-6" />,
    },
    {
      label: "Budgets",
      href: "/budgets",
      description: "View and manage budget plans.",
      icon: <Wallet className="h-6 w-6" />,
    },
    {
      label: "Expenses",
      href: "/expenses",
      description: "Track purchase orders and receipts.",
      icon: <Receipt className="h-6 w-6" />,
    },
    {
      label: "Inventory",
      href: "/inventory",
      description: "Equipment, props, and locations.",
      icon: <Package className="h-6 w-6" />,
    },
    {
      label: "Reports",
      href: "/reports",
      description: "Cross-service analytics and reports.",
      icon: <BarChart3 className="h-6 w-6" />,
    },
    {
      label: entityLabels.teamMemberPlural,
      href: "/team",
      description: `View ${entityLabels.teamMemberPlural.toLowerCase()} and assignments.`,
      icon: <Users className="h-6 w-6" />,
    },
  ];

  return (
    <div className="mx-auto max-w-5xl">
      <div className="mb-8">
        <h1
          className="text-2xl font-semibold tracking-tight"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          Welcome to your {entityLabels.projectPlural}
          {user ? `, ${user.displayName}` : ""}
        </h1>
        <p
          className="mt-1 text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Here is an overview of your workspace. Pick up where you left off.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {quickCards.map((card) => (
          <Link
            key={card.href}
            href={card.href}
            className="group flex flex-col gap-3 rounded-xl p-5 transition-shadow hover:shadow-md"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <div
              className="flex h-10 w-10 items-center justify-center rounded-lg"
              style={{
                backgroundColor:
                  "color-mix(in srgb, var(--thittam-primary, #3b82f6) 10%, transparent)",
                color: "var(--thittam-primary, #3b82f6)",
              }}
            >
              {card.icon}
            </div>
            <div>
              <h2
                className="text-sm font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {card.label}
              </h2>
              <p
                className="mt-0.5 text-xs"
                style={{
                  color: "var(--thittam-muted-foreground, #64748b)",
                }}
              >
                {card.description}
              </p>
            </div>
          </Link>
        ))}
      </div>

      {/* Placeholder for Phase 6 dashboard widgets */}
      <div
        className="mt-10 rounded-xl border-dashed p-8 text-center"
        style={{
          border: "2px dashed var(--thittam-border, #e2e8f0)",
        }}
      >
        <p
          className="text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Dashboard widgets will appear here in a future release.
        </p>
      </div>
    </div>
  );
}
