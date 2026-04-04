"use client";

import { useState, useMemo } from "react";
import { useRouter } from "next/navigation";
import { useTheme } from "@/lib/themes/provider";
import { PageHeader } from "@/components/ui/page-header";
import { StatusBadge } from "@/components/ui/status-badge";
import { DataTable, type Column } from "@/components/ui/data-table";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type MemberStatus = "active" | "on_leave" | "offboarded";

interface TeamMember {
  id: string;
  name: string;
  role: string;
  department: string;
  day_rate: string;
  currency: string;
  status: MemberStatus;
  current_project: string | null;
  email: string;
  joined_at: string;
}

// ---------------------------------------------------------------------------
// Mock data — 8 team members from XYZ_CBA
// ---------------------------------------------------------------------------

const mockTeamMembers: TeamMember[] = [
  {
    id: "tm-001",
    name: "Arjun Mehta",
    role: "Director of Photography",
    department: "Camera",
    day_rate: "75000.00",
    currency: "INR",
    status: "active",
    current_project: "The Last Horizon",
    email: "arjun.mehta@xyzcba.com",
    joined_at: "2025-07-01",
  },
  {
    id: "tm-002",
    name: "Priya Sharma",
    role: "Executive Producer",
    department: "Production Office",
    day_rate: "100000.00",
    currency: "INR",
    status: "active",
    current_project: "The Last Horizon",
    email: "priya.sharma@xyzcba.com",
    joined_at: "2025-06-15",
  },
  {
    id: "tm-003",
    name: "Ravi Kumar",
    role: "Art Director",
    department: "Art",
    day_rate: "50000.00",
    currency: "INR",
    status: "active",
    current_project: "The Last Horizon",
    email: "ravi.kumar@xyzcba.com",
    joined_at: "2025-09-01",
  },
  {
    id: "tm-004",
    name: "Deepa Nair",
    role: "Production Accountant",
    department: "Accounts",
    day_rate: "45000.00",
    currency: "INR",
    status: "active",
    current_project: "The Last Horizon",
    email: "deepa.nair@xyzcba.com",
    joined_at: "2025-07-10",
  },
  {
    id: "tm-005",
    name: "Kiran Rao",
    role: "Production Manager",
    department: "Production Office",
    day_rate: "55000.00",
    currency: "INR",
    status: "active",
    current_project: "The Last Horizon",
    email: "kiran.rao@xyzcba.com",
    joined_at: "2025-08-05",
  },
  {
    id: "tm-006",
    name: "Sanjay Kapoor",
    role: "Gaffer",
    department: "Lighting",
    day_rate: "35000.00",
    currency: "INR",
    status: "on_leave",
    current_project: null,
    email: "sanjay.kapoor@xyzcba.com",
    joined_at: "2025-10-01",
  },
  {
    id: "tm-007",
    name: "Meena Iyer",
    role: "Costume Designer",
    department: "Wardrobe",
    day_rate: "40000.00",
    currency: "INR",
    status: "active",
    current_project: "The Last Horizon",
    email: "meena.iyer@xyzcba.com",
    joined_at: "2025-11-15",
  },
  {
    id: "tm-008",
    name: "Vikram Singh",
    role: "Stunt Coordinator",
    department: "Action",
    day_rate: "60000.00",
    currency: "INR",
    status: "offboarded",
    current_project: null,
    email: "vikram.singh@xyzcba.com",
    joined_at: "2025-08-20",
  },
];

// ---------------------------------------------------------------------------
// Filter options
// ---------------------------------------------------------------------------

const STATUS_OPTIONS = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "on_leave", label: "On Leave" },
  { key: "offboarded", label: "Offboarded" },
] as const;

type StatusFilter = (typeof STATUS_OPTIONS)[number]["key"];

const DEPARTMENT_OPTIONS = [
  "All Departments",
  ...Array.from(new Set(mockTeamMembers.map((m) => m.department))).sort(),
];

// ---------------------------------------------------------------------------
// Indian number formatting for day rate
// ---------------------------------------------------------------------------

function formatIndian(num: number): string {
  const [intPart, decPart] = Math.abs(num).toFixed(2).split(".");
  const sign = num < 0 ? "-" : "";

  if (intPart.length <= 3) {
    return `${sign}${intPart}.${decPart}`;
  }

  const last3 = intPart.slice(-3);
  const rest = intPart.slice(0, -3);
  const grouped = rest.replace(/\B(?=(\d{2})+(?!\d))/g, ",");

  return `${sign}${grouped},${last3}.${decPart}`;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function TeamPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [departmentFilter, setDepartmentFilter] = useState("All Departments");

  const filtered = useMemo(() => {
    return mockTeamMembers.filter((m) => {
      if (statusFilter !== "all" && m.status !== statusFilter) return false;
      if (
        departmentFilter !== "All Departments" &&
        m.department !== departmentFilter
      )
        return false;
      return true;
    });
  }, [statusFilter, departmentFilter]);

  const columns: Column<TeamMember>[] = [
    {
      key: "name",
      header: "Name",
      sortable: true,
      render: (row) => (
        <span
          className="font-heading text-sm font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.name}
        </span>
      ),
    },
    {
      key: "role",
      header: "Role",
      sortable: true,
      render: (row) => (
        <span
          className="font-body text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.role}
        </span>
      ),
    },
    {
      key: "department",
      header: "Department",
      sortable: true,
      render: (row) => (
        <span
          className="font-heading text-xs font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.department}
        </span>
      ),
    },
    {
      key: "day_rate",
      header: entityLabels.rateLabel,
      type: "number",
      sortable: true,
      render: (row) => (
        <span
          className="font-mono tabular-nums text-sm"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
          data-monetary
        >
          {"\u20B9"}
          {formatIndian(parseFloat(row.day_rate))}
          <span
            className="ml-1 text-[10px]"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            /{entityLabels.rateUnit}
          </span>
        </span>
      ),
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: "current_project",
      header: `Current ${entityLabels.project}`,
      render: (row) =>
        row.current_project ? (
          <span
            className="font-body text-sm"
            style={{ color: "var(--thittam-primary, #3b82f6)" }}
          >
            {row.current_project}
          </span>
        ) : (
          <span
            className="font-body text-sm"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {"\u2014"}
          </span>
        ),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <span
          className="cursor-pointer text-xs font-heading font-medium transition-colors hover:opacity-80"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
        >
          View
        </span>
      ),
    },
  ];

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title={entityLabels.teamMemberPlural}
        description={`Manage ${entityLabels.teamMemberPlural.toLowerCase()} and their assignments across ${entityLabels.projectPlural.toLowerCase()}`}
        icon="Users"
      />

      {/* Filters */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        {/* Status filter chips */}
        <div className="flex flex-wrap gap-2">
          {STATUS_OPTIONS.map((opt) => (
            <button
              key={opt.key}
              onClick={() => setStatusFilter(opt.key)}
              className={`rounded-full px-3 py-1 text-xs font-heading font-medium transition-colors ${
                statusFilter === opt.key ? "text-white" : ""
              }`}
              style={
                statusFilter === opt.key
                  ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
                  : {
                      backgroundColor: "var(--thittam-muted, #f1f5f9)",
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }
              }
            >
              {opt.label}
            </button>
          ))}
        </div>

        {/* Department dropdown */}
        <select
          value={departmentFilter}
          onChange={(e) => setDepartmentFilter(e.target.value)}
          className="rounded-lg px-3 py-1.5 text-sm font-heading"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
          }}
        >
          {DEPARTMENT_OPTIONS.map((d) => (
            <option key={d} value={d}>
              {d}
            </option>
          ))}
        </select>
      </div>

      {/* Table */}
      <DataTable<Record<string, unknown>>
        columns={columns as unknown as Column<Record<string, unknown>>[]}
        data={filtered as unknown as Record<string, unknown>[]}
        emptyMessage={`No ${entityLabels.teamMemberPlural.toLowerCase()} match the selected filters.`}
      />
    </div>
  );
}
