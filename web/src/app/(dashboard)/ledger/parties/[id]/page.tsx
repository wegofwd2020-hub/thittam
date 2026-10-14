"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { PageHeader } from "@/components/ui/page-header";
import { DataTable, type Column } from "@/components/ui/data-table";
import {
  listParties,
  listPartyTransactions,
  type Party,
} from "@/lib/api/ledger";

const TYPE_LABEL: Record<string, string> = {
  customer: "Customer",
  supplier: "Vendor",
  both: "Customer & vendor",
};

// View-only report of posted ledger lines tagged to one customer or vendor.
export default function PartyTransactionsPage() {
  const params = useParams<{ id: string }>();
  const partyId = params.id;

  const partiesQuery = useQuery({
    queryKey: ["ledger", "parties", "all"],
    queryFn: () => listParties({ include_inactive: true }),
  });
  const txQuery = useQuery({
    queryKey: ["ledger", "parties", partyId, "transactions"],
    queryFn: () => listPartyTransactions(partyId),
    enabled: !!partyId,
  });

  const party: Party | undefined = partiesQuery.data?.parties.find((p) => p.id === partyId);

  const rows = (txQuery.data?.lines ?? []).map((l) => ({
    entry_date: l.entry_date.slice(0, 10),
    entry_number: l.entry_number,
    account: `${l.account_code} ${l.account_name}`,
    narration: l.narration,
    description: l.description,
    debit: l.debit_amount,
    credit: l.credit_amount,
  }));

  const columns: Column<Record<string, unknown>>[] = [
    { key: "entry_date", header: "Date" },
    { key: "entry_number", header: "Entry #" },
    { key: "account", header: "Account" },
    { key: "narration", header: "Narration" },
    { key: "description", header: "Line description" },
    { key: "debit", header: "Debit", type: "number" },
    { key: "credit", header: "Credit", type: "number" },
  ];

  const errorMessage =
    txQuery.error?.message ?? partiesQuery.error?.message ?? null;

  return (
    <div className="space-y-6">
      <Link href="/ledger" className="text-sm text-muted-foreground hover:underline">
        ← Ledger
      </Link>

      <PageHeader
        title={party?.name ?? "Customer or vendor"}
        description={
          party
            ? `${TYPE_LABEL[party.party_type] ?? party.party_type}${party.default_currency ? ` · ${party.default_currency}` : ""}`
            : "Posted transactions tagged to this party."
        }
        icon="BookOpen"
      />

      {errorMessage && <p className="text-sm text-red-700">Failed to load: {errorMessage}</p>}

      {txQuery.data && (
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="rounded-lg border p-4">
            <p className="text-xs text-muted-foreground">Total debit</p>
            <p className="font-mono text-lg">{txQuery.data.total_debit}</p>
          </div>
          <div className="rounded-lg border p-4">
            <p className="text-xs text-muted-foreground">Total credit</p>
            <p className="font-mono text-lg">{txQuery.data.total_credit}</p>
          </div>
        </div>
      )}

      <DataTable<Record<string, unknown>>
        columns={columns}
        data={rows}
        isLoading={txQuery.isLoading}
        emptyMessage="No posted transactions tagged to this party yet."
      />
    </div>
  );
}
