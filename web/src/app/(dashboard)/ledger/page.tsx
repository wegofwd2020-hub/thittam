"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PageHeader } from "@/components/ui/page-header";
import { DataTable, type Column } from "@/components/ui/data-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusBadge } from "@/components/ui/status-badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  createJournalEntry,
  fromCents,
  getTrialBalance,
  listAccountingPeriods,
  listJournalEntries,
  listLedgerAccounts,
  postJournalEntry,
  toCents,
  type JournalLineInput,
} from "@/lib/api/ledger";

// The ledger stores currency per line; this tenant books in USD.
const CURRENCY = "USD";
const AMOUNT_RE = /^\d+(\.\d{1,2})?$/;

const ledgerKeys = {
  accounts: ["ledger", "accounts"] as const,
  periods: ["ledger", "periods"] as const,
  entries: ["ledger", "entries"] as const,
  trial: ["ledger", "trial-balance"] as const,
};

interface DraftLine {
  account_id: string;
  debit: string;
  credit: string;
  description: string;
}

const EMPTY_LINE: DraftLine = { account_id: "", debit: "", credit: "", description: "" };

export default function LedgerPage() {
  const queryClient = useQueryClient();

  const accountsQuery = useQuery({
    queryKey: ledgerKeys.accounts,
    queryFn: listLedgerAccounts,
  });
  const periodsQuery = useQuery({
    queryKey: ledgerKeys.periods,
    queryFn: listAccountingPeriods,
  });
  const entriesQuery = useQuery({
    queryKey: ledgerKeys.entries,
    queryFn: () => listJournalEntries({ limit: 200 }),
  });
  const trialQuery = useQuery({
    queryKey: ledgerKeys.trial,
    queryFn: getTrialBalance,
  });

  const accounts = accountsQuery.data?.accounts ?? [];

  const openPeriods = (periodsQuery.data?.periods ?? [])
    .filter((p) => p.status === "open")
    .sort((a, b) => a.year - b.year || a.month - b.month);

  // ---- Entry form state ----------------------------------------------------
  const [periodId, setPeriodId] = useState("");
  const [narration, setNarration] = useState("");
  const [reference, setReference] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([{ ...EMPTY_LINE }, { ...EMPTY_LINE }]);
  const [formError, setFormError] = useState<string | null>(null);

  const selectedPeriod = periodId || openPeriods[0]?.id || "";

  const totals = useMemo(() => {
    const debit = lines.reduce((s, l) => s + toCents(l.debit), 0);
    const credit = lines.reduce((s, l) => s + toCents(l.credit), 0);
    return { debit, credit, diff: debit - credit };
  }, [lines]);

  function updateLine(i: number, patch: Partial<DraftLine>) {
    setLines((prev) => prev.map((l, idx) => (idx === i ? { ...l, ...patch } : l)));
  }

  function buildPayload(): JournalLineInput[] | string {
    // Matches the server rules: >= 2 lines, each line has exactly one side > 0,
    // accounts chosen, and debits equal credits.
    if (!selectedPeriod) return "No open accounting period.";
    if (!narration.trim()) return "Narration is required.";
    if (lines.length < 2) return "An entry needs at least two lines.";
    const out: JournalLineInput[] = [];
    for (const [i, l] of lines.entries()) {
      const n = i + 1;
      if (!l.account_id) return `Line ${n}: choose an account.`;
      const d = l.debit.trim() || "0";
      const c = l.credit.trim() || "0";
      if (!AMOUNT_RE.test(d) || !AMOUNT_RE.test(c)) return `Line ${n}: amounts must be like 500.00.`;
      const dc = toCents(d);
      const cc = toCents(c);
      if ((dc > 0) === (cc > 0)) return `Line ${n}: enter an amount on exactly one side.`;
      out.push({
        account_id: l.account_id,
        debit_amount: fromCents(dc),
        credit_amount: fromCents(cc),
        currency: CURRENCY,
        description: l.description.trim(),
      });
    }
    if (totals.diff !== 0) return `Entry is not balanced (difference ${fromCents(totals.diff)}).`;
    return out;
  }

  const saveMutation = useMutation({
    mutationFn: async (andPost: boolean) => {
      const payload = buildPayload();
      if (typeof payload === "string") throw new Error(payload);
      const entry = await createJournalEntry({
        period_id: selectedPeriod,
        narration: narration.trim(),
        reference: reference.trim() || undefined,
        lines: payload,
      });
      if (andPost) return postJournalEntry(entry.id);
      return entry;
    },
    onSuccess: () => {
      setFormError(null);
      setNarration("");
      setReference("");
      setLines([{ ...EMPTY_LINE }, { ...EMPTY_LINE }]);
      queryClient.invalidateQueries({ queryKey: ["ledger"] });
    },
    onError: (err: Error) => setFormError(err.message),
  });

  // ---- Table rows ----------------------------------------------------------
  const accountRows = accounts.map((a) => ({
    code: a.code,
    name: a.name,
    account_type: a.account_type,
    status: a.is_active ? "active" : "inactive",
  }));

  const entryRows = (entriesQuery.data?.entries ?? []).map((e) => ({
    entry_number: e.entry_number,
    narration: e.narration,
    status: e.status,
    amount: fromCents(e.lines.reduce((s, l) => s + toCents(l.debit_amount), 0)),
    lines: e.lines.length,
  }));

  const trialRows = (trialQuery.data?.entries ?? []).map((t) => ({
    code: t.code,
    name: t.name,
    debit: t.debit,
    credit: t.credit,
    balance: t.balance,
  }));

  const accountColumns: Column<Record<string, unknown>>[] = [
    { key: "code", header: "Code" },
    { key: "name", header: "Account" },
    { key: "account_type", header: "Type" },
    { key: "status", header: "Status", type: "status" },
  ];
  const entryColumns: Column<Record<string, unknown>>[] = [
    { key: "entry_number", header: "Entry #" },
    { key: "narration", header: "Narration" },
    {
      key: "status",
      header: "Status",
      render: (row) => <StatusBadge status={String(row.status)} />,
    },
    { key: "amount", header: "Amount", type: "number" },
    { key: "lines", header: "Lines", type: "number" },
  ];
  const trialColumns: Column<Record<string, unknown>>[] = [
    { key: "code", header: "Code" },
    { key: "name", header: "Account" },
    { key: "debit", header: "Debit", type: "number" },
    { key: "credit", header: "Credit", type: "number" },
    { key: "balance", header: "Balance", type: "number" },
  ];

  const selectClass =
    "h-9 w-full rounded-md border border-input bg-background px-2 text-sm";

  return (
    <div className="space-y-6">
      <PageHeader
        title="Ledger"
        description="Chart of accounts, journal entries and trial balance for this company."
        icon="BookOpen"
      />

      <Tabs defaultValue="entries">
        <TabsList>
          <TabsTrigger value="entries">Journal entries</TabsTrigger>
          <TabsTrigger value="accounts">Accounts</TabsTrigger>
          <TabsTrigger value="trial">Trial balance</TabsTrigger>
        </TabsList>

        <TabsContent value="accounts" className="pt-4">
          <DataTable<Record<string, unknown>>
            columns={accountColumns}
            data={accountRows}
            isLoading={accountsQuery.isLoading}
            emptyMessage={
              accountsQuery.error
                ? `Failed to load accounts: ${accountsQuery.error.message}`
                : "No accounts yet."
            }
          />
        </TabsContent>

        <TabsContent value="entries" className="space-y-6 pt-4">
          <section className="rounded-lg border p-4 space-y-4">
            <h2 className="font-medium">New journal entry</h2>

            <div className="grid gap-3 md:grid-cols-3">
              <label className="space-y-1 text-sm">
                <span>Period</span>
                <select
                  className={selectClass}
                  value={selectedPeriod}
                  onChange={(e) => setPeriodId(e.target.value)}
                >
                  {openPeriods.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.year}-{String(p.month).padStart(2, "0")} (open)
                    </option>
                  ))}
                </select>
              </label>
              <label className="space-y-1 text-sm md:col-span-2">
                <span>Narration</span>
                <Input
                  value={narration}
                  onChange={(e) => setNarration(e.target.value)}
                  placeholder="e.g. Owner's opening contribution"
                />
              </label>
              <label className="space-y-1 text-sm">
                <span>Reference (optional)</span>
                <Input value={reference} onChange={(e) => setReference(e.target.value)} />
              </label>
            </div>

            <div className="space-y-2">
              <div className="grid grid-cols-12 gap-2 text-xs font-medium text-muted-foreground">
                <span className="col-span-4">Account</span>
                <span className="col-span-2">Debit (USD)</span>
                <span className="col-span-2">Credit (USD)</span>
                <span className="col-span-3">Description</span>
                <span className="col-span-1" />
              </div>
              {lines.map((l, i) => (
                <div key={i} className="grid grid-cols-12 items-center gap-2">
                  <select
                    className={`${selectClass} col-span-4`}
                    value={l.account_id}
                    onChange={(e) => updateLine(i, { account_id: e.target.value })}
                  >
                    <option value="">Choose account…</option>
                    {accounts
                      .filter((a) => a.is_active)
                      .map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.code} {a.name}
                        </option>
                      ))}
                  </select>
                  <Input
                    className="col-span-2"
                    inputMode="decimal"
                    placeholder="0.00"
                    value={l.debit}
                    onChange={(e) => updateLine(i, { debit: e.target.value })}
                  />
                  <Input
                    className="col-span-2"
                    inputMode="decimal"
                    placeholder="0.00"
                    value={l.credit}
                    onChange={(e) => updateLine(i, { credit: e.target.value })}
                  />
                  <Input
                    className="col-span-3"
                    value={l.description}
                    onChange={(e) => updateLine(i, { description: e.target.value })}
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    className="col-span-1"
                    disabled={lines.length <= 2}
                    onClick={() => setLines((prev) => prev.filter((_, idx) => idx !== i))}
                  >
                    Remove
                  </Button>
                </div>
              ))}
              <Button
                variant="outline"
                size="sm"
                onClick={() => setLines((prev) => [...prev, { ...EMPTY_LINE }])}
              >
                Add line
              </Button>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
              <span>
                Debits {fromCents(totals.debit)} · Credits {fromCents(totals.credit)} ·{" "}
                {totals.diff === 0 ? (
                  <span className="text-green-700">balanced</span>
                ) : (
                  <span className="text-red-700">difference {fromCents(totals.diff)}</span>
                )}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  disabled={saveMutation.isPending}
                  onClick={() => saveMutation.mutate(false)}
                >
                  Save draft
                </Button>
                <Button
                  disabled={saveMutation.isPending}
                  onClick={() => saveMutation.mutate(true)}
                >
                  {saveMutation.isPending ? "Posting…" : "Post entry"}
                </Button>
              </div>
            </div>
            {formError && <p className="text-sm text-red-700">{formError}</p>}
          </section>

          <DataTable<Record<string, unknown>>
            columns={entryColumns}
            data={entryRows}
            isLoading={entriesQuery.isLoading}
            emptyMessage={
              entriesQuery.error
                ? `Failed to load journal entries: ${entriesQuery.error.message}`
                : "No journal entries yet."
            }
          />
        </TabsContent>

        <TabsContent value="trial" className="pt-4">
          <DataTable<Record<string, unknown>>
            columns={trialColumns}
            data={trialRows}
            isLoading={trialQuery.isLoading}
            emptyMessage={
              trialQuery.error
                ? `Failed to load trial balance: ${trialQuery.error.message}`
                : "No posted entries yet."
            }
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
