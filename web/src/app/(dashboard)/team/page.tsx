"use client";

import { useMemo, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { PageHeader } from "@/components/ui/page-header";
import { DataTable, type Column } from "@/components/ui/data-table";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { useProductions, productionKeys } from "@/lib/hooks/use-productions";
import { useUsers } from "@/lib/hooks/use-settings";
import { listCrewMembers, type CrewMember } from "@/lib/api/productions";

type CrewRow = CrewMember & { productionTitle: string };

const crewColumns: Column<Record<string, unknown>>[] = [
  { key: "name", header: "Name", sortable: true },
  { key: "role", header: "Role", sortable: true },
  { key: "department", header: "Department", sortable: true },
  { key: "productionTitle", header: "Production", sortable: true },
  {
    key: "day_rate",
    header: "Day rate",
    type: "number",
    render: (row) => (
      <AmountDisplay amount={String(row.day_rate)} currency={String(row.currency)} size="sm" />
    ),
  },
];

const userColumns: Column<Record<string, unknown>>[] = [
  { key: "display_name", header: "Name", sortable: true },
  { key: "email", header: "Email", sortable: true },
  {
    key: "status",
    header: "Status",
    render: (row) => <StatusBadge status={String(row.status)} />,
  },
  { key: "created_at", header: "Joined", type: "date", sortable: true },
];

export default function TeamPage() {
  const [departmentFilter, setDepartmentFilter] = useState("All Departments");

  const productionsQuery = useProductions();
  const productions = productionsQuery.data?.productions ?? [];

  // Crew is stored per production; fetch each and combine.
  const crewQueries = useQueries({
    queries: productions.map((p) => ({
      queryKey: productionKeys.crew(p.id),
      queryFn: () => listCrewMembers(p.id),
    })),
  });

  const crew: CrewRow[] = crewQueries.flatMap((q, i) =>
    (q.data ?? []).map((m) => ({ ...m, productionTitle: productions[i]?.title ?? "" })),
  );
  const crewLoading = crewQueries.some((q) => q.isLoading);
  const crewError = crewQueries.find((q) => q.error)?.error;

  const departments = useMemo(
    () => ["All Departments", ...Array.from(new Set(crew.map((m) => m.department).filter(Boolean))).sort()],
    [crew],
  );
  const filteredCrew = crew.filter(
    (m) => departmentFilter === "All Departments" || m.department === departmentFilter,
  );

  const usersQuery = useUsers();
  const users = usersQuery.data?.users ?? [];

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title="Team"
        description="Crew working on your productions, and people with access to this workspace"
        icon="Users"
      />

      <Tabs defaultValue="crew">
        <TabsList>
          <TabsTrigger value="crew">Crew</TabsTrigger>
          <TabsTrigger value="users">Users</TabsTrigger>
        </TabsList>

        <TabsContent value="crew" className="space-y-4 pt-4">
          <div>
            <select
              aria-label="Filter by department"
              value={departmentFilter}
              onChange={(e) => setDepartmentFilter(e.target.value)}
              className="h-9 rounded-md border border-input bg-background px-2 text-sm"
            >
              {departments.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          </div>
          <DataTable<Record<string, unknown>>
            columns={crewColumns}
            data={filteredCrew as unknown as Record<string, unknown>[]}
            isLoading={productionsQuery.isLoading || crewLoading}
            emptyMessage={
              crewError
                ? `Failed to load crew: ${crewError.message}`
                : "No crew members yet."
            }
          />
        </TabsContent>

        <TabsContent value="users" className="pt-4">
          <DataTable<Record<string, unknown>>
            columns={userColumns}
            data={users as unknown as Record<string, unknown>[]}
            isLoading={usersQuery.isLoading}
            emptyMessage={
              usersQuery.error
                ? `Failed to load users: ${usersQuery.error.message}`
                : "No users found."
            }
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
