"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PageHeader } from "@/components/ui/page-header";
import { DataTable, type Column } from "@/components/ui/data-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusBadge } from "@/components/ui/status-badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { PartyRolodex } from "@/components/ledger/party-rolodex";
import { CostBehaviorReport } from "@/components/ledger/cost-behavior-report";
import {
  createBankAccount,
  createJournalEntry,
  createParty,
  listParties,
  updateParty,
  type Party,
  type PartyContactInput,
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
  parties: ["ledger", "parties"] as const,
};

const BLANK_CONTACT: PartyContactInput = {
  name: "",
  designation: "",
  email: "",
  phone: "",
  is_primary: false,
};

const PARTY_TYPES = [
  { value: "customer", label: "Customer" },
  { value: "supplier", label: "Vendor" },
  { value: "both", label: "Customer & vendor" },
];

interface DraftLine {
  account_id: string;
  debit: string;
  credit: string;
  description: string;
  party_id: string;
}

const EMPTY_LINE: DraftLine = { account_id: "", debit: "", credit: "", description: "", party_id: "" };

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
    // The service caps limit at 100 and silently falls back to 20 above that.
    queryFn: () => listJournalEntries({ limit: 100 }),
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
  const [postError, setPostError] = useState<string | null>(null);

  // ---- Bank account form state ---------------------------------------------
  const [showBankForm, setShowBankForm] = useState(false);
  const [bankName, setBankName] = useState("");
  const [bankNumber, setBankNumber] = useState("");
  const [bankCurrency, setBankCurrency] = useState("USD");
  const [bankError, setBankError] = useState<string | null>(null);

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
        ...(l.party_id ? { party_id: l.party_id } : {}),
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

  // Posts an existing draft. The service validates balance and period, so a
  // failure comes back as the error text shown above the table.
  const postEntryMutation = useMutation({
    mutationFn: (id: string) => postJournalEntry(id),
    onMutate: () => setPostError(null),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["ledger"] }),
    onError: (err: Error) => setPostError(err.message),
  });

  const bankMutation = useMutation({
    mutationFn: () =>
      createBankAccount({
        bank_name: bankName.trim(),
        account_number: bankNumber.trim(),
        bank_currency: bankCurrency.trim().toUpperCase(),
      }),
    onSuccess: () => {
      setBankError(null);
      setBankName("");
      setBankNumber("");
      setBankCurrency("USD");
      setShowBankForm(false);
      queryClient.invalidateQueries({ queryKey: ledgerKeys.accounts });
    },
    onError: (err: Error) => setBankError(err.message),
  });

  // ---- Customers & vendors -------------------------------------------------
  const [showPartyForm, setShowPartyForm] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const [partyView, setPartyView] = useState<"rolodex" | "table">("rolodex");
  const [partyName, setPartyName] = useState("");
  const [partyType, setPartyType] = useState("customer");
  const [partyEmail, setPartyEmail] = useState("");
  const [partyPhone, setPartyPhone] = useState("");
  const [partyTaxId, setPartyTaxId] = useState("");
  const [partyAddress, setPartyAddress] = useState("");
  const [partyCurrency, setPartyCurrency] = useState("");
  const [partyContacts, setPartyContacts] = useState<PartyContactInput[]>([{ ...BLANK_CONTACT }]);
  const [partyError, setPartyError] = useState<string | null>(null);

  const partiesQuery = useQuery({
    queryKey: [...ledgerKeys.parties, showInactive],
    queryFn: () => listParties({ include_inactive: showInactive }),
  });

  function resetPartyForm() {
    setPartyName("");
    setPartyType("customer");
    setPartyEmail("");
    setPartyPhone("");
    setPartyTaxId("");
    setPartyAddress("");
    setPartyCurrency("");
    setPartyContacts([{ ...BLANK_CONTACT }]);
  }

  function updateContact(i: number, patch: Partial<PartyContactInput>) {
    setPartyContacts((prev) => prev.map((c, idx) => (idx === i ? { ...c, ...patch } : c)));
  }

  function setPrimaryContact(i: number) {
    setPartyContacts((prev) => prev.map((c, idx) => ({ ...c, is_primary: idx === i })));
  }

  const partyMutation = useMutation({
    mutationFn: () =>
      createParty({
        name: partyName,
        party_type: partyType,
        email: partyEmail,
        phone: partyPhone,
        tax_id: partyTaxId,
        address: partyAddress,
        default_currency: partyCurrency,
        // Drop rows left blank; the server requires a name and an email or phone.
        contacts: partyContacts.filter((c) => c.name.trim() || c.email.trim() || c.phone.trim()),
      }),
    onSuccess: () => {
      setPartyError(null);
      resetPartyForm();
      setShowPartyForm(false);
      queryClient.invalidateQueries({ queryKey: ledgerKeys.parties });
    },
    onError: (err: Error) => setPartyError(err.message),
  });

  const togglePartyMutation = useMutation({
    mutationFn: (p: Party) =>
      updateParty(p.id, {
        name: p.name,
        party_type: p.party_type,
        email: p.email,
        phone: p.phone,
        tax_id: p.tax_id,
        address: p.address,
        default_currency: p.default_currency,
        contacts: p.contacts.map(({ name, designation, email, phone, is_primary }) => ({
          name,
          designation,
          email,
          phone,
          is_primary,
        })),
        is_active: !p.is_active,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ledgerKeys.parties }),
    onError: (err: Error) => setPartyError(err.message),
  });

  const partyRows = (partiesQuery.data?.parties ?? []).map((p) => {
    const primary = p.contacts.find((c) => c.is_primary) ?? p.contacts[0];
    return {
      id: p.id,
      name: p.name,
      party_type: PARTY_TYPES.find((t) => t.value === p.party_type)?.label ?? p.party_type,
      contact: primary
        ? [primary.name, primary.designation].filter(Boolean).join(", ")
        : "",
      email: p.email,
      currency: p.default_currency,
      status: p.is_active ? "active" : "inactive",
      raw: p,
    };
  });

  // ---- Table rows ----------------------------------------------------------
  const accountRows = accounts.map((a) => ({
    code: a.code,
    name: a.name,
    account_type: a.account_type,
    currency: a.bank_currency,
    status: a.is_active ? "active" : "inactive",
  }));

  const entryRows = (entriesQuery.data?.entries ?? []).map((e) => ({
    id: e.id,
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

  const partyColumns: Column<Record<string, unknown>>[] = [
    { key: "name", header: "Name" },
    { key: "party_type", header: "Type" },
    { key: "contact", header: "Primary contact" },
    { key: "email", header: "Email" },
    { key: "currency", header: "Currency" },
    { key: "status", header: "Status", type: "status" },
    {
      key: "id",
      header: "",
      render: (row) => {
        const p = row.raw as Party;
        return (
          <Button
            variant="ghost"
            size="sm"
            disabled={togglePartyMutation.isPending}
            onClick={() => togglePartyMutation.mutate(p)}
          >
            {p.is_active ? "Deactivate" : "Reactivate"}
          </Button>
        );
      },
    },
  ];

  const accountColumns: Column<Record<string, unknown>>[] = [
    { key: "code", header: "Code" },
    { key: "name", header: "Account" },
    { key: "account_type", header: "Type" },
    { key: "currency", header: "Currency" },
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
    {
      key: "action",
      header: "",
      render: (row) =>
        row.status === "draft" ? (
          <Button
            variant="outline"
            size="sm"
            disabled={postEntryMutation.isPending}
            onClick={() => postEntryMutation.mutate(String(row.id))}
          >
            Post
          </Button>
        ) : null,
    },
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
          <TabsTrigger value="cost-behavior">Fixed vs variable</TabsTrigger>
          <TabsTrigger value="parties">Customers &amp; vendors</TabsTrigger>
        </TabsList>

        <TabsContent value="accounts" className="space-y-4 pt-4">
          <div className="flex justify-end">
            <Button variant="outline" size="sm" onClick={() => setShowBankForm((v) => !v)}>
              {showBankForm ? "Cancel" : "New bank account"}
            </Button>
          </div>

          {showBankForm && (
            <section className="rounded-lg border p-4 space-y-4">
              <h2 className="font-medium">New bank account</h2>
              <div className="grid gap-3 md:grid-cols-3">
                <label className="space-y-1 text-sm">
                  <span>Bank name</span>
                  <Input
                    value={bankName}
                    onChange={(e) => setBankName(e.target.value)}
                    placeholder="e.g. HDFC Bank"
                  />
                </label>
                <label className="space-y-1 text-sm">
                  <span>Account number</span>
                  <Input
                    inputMode="numeric"
                    autoComplete="off"
                    value={bankNumber}
                    onChange={(e) => setBankNumber(e.target.value)}
                    placeholder="Only the last 4 digits are stored"
                  />
                </label>
                <label className="space-y-1 text-sm">
                  <span>Currency</span>
                  <Input
                    maxLength={3}
                    value={bankCurrency}
                    onChange={(e) => setBankCurrency(e.target.value)}
                  />
                </label>
              </div>
              <div className="flex items-center justify-between gap-3">
                {bankError ? (
                  <p className="text-sm text-red-700">{bankError}</p>
                ) : (
                  <span />
                )}
                <Button
                  disabled={bankMutation.isPending}
                  onClick={() => bankMutation.mutate()}
                >
                  {bankMutation.isPending ? "Saving…" : "Save bank account"}
                </Button>
              </div>
            </section>
          )}

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
                <span className="col-span-3">Account</span>
                <span className="col-span-2">Party</span>
                <span className="col-span-2">Debit (USD)</span>
                <span className="col-span-2">Credit (USD)</span>
                <span className="col-span-2">Description</span>
                <span className="col-span-1" />
              </div>
              {lines.map((l, i) => (
                <div key={i} className="grid grid-cols-12 items-center gap-2">
                  <select
                    className={`${selectClass} col-span-3`}
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
                  <select
                    className={`${selectClass} col-span-2`}
                    aria-label="Party (optional)"
                    value={l.party_id}
                    onChange={(e) => updateLine(i, { party_id: e.target.value })}
                  >
                    <option value="">No party</option>
                    {(partiesQuery.data?.parties ?? [])
                      .filter((p) => p.is_active)
                      .map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
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
                    className="col-span-2"
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

          {postError && <p className="text-sm text-red-700">{postError}</p>}
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

        <TabsContent value="parties" className="space-y-4 pt-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={showInactive}
                onChange={(e) => setShowInactive(e.target.checked)}
              />
              Show inactive
            </label>
            <div className="flex items-center gap-2">
              <div className="flex gap-1">
                <Button
                  size="sm"
                  variant={partyView === "rolodex" ? "default" : "outline"}
                  onClick={() => setPartyView("rolodex")}
                >
                  Rolodex
                </Button>
                <Button
                  size="sm"
                  variant={partyView === "table" ? "default" : "outline"}
                  onClick={() => setPartyView("table")}
                >
                  Table
                </Button>
              </div>
              <Button variant="outline" size="sm" onClick={() => setShowPartyForm((v) => !v)}>
                {showPartyForm ? "Cancel" : "New customer or vendor"}
              </Button>
            </div>
          </div>

          {showPartyForm && (
            <section className="rounded-lg border p-4 space-y-4">
              <h2 className="font-medium">New customer or vendor</h2>
              <div className="grid gap-3 md:grid-cols-3">
                <label className="space-y-1 text-sm">
                  <span>Name</span>
                  <Input value={partyName} onChange={(e) => setPartyName(e.target.value)} />
                </label>
                <label className="space-y-1 text-sm">
                  <span>Type</span>
                  <select
                    className={selectClass}
                    value={partyType}
                    onChange={(e) => setPartyType(e.target.value)}
                  >
                    {PARTY_TYPES.map((t) => (
                      <option key={t.value} value={t.value}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="space-y-1 text-sm">
                  <span>Default currency (optional)</span>
                  <Input
                    maxLength={3}
                    value={partyCurrency}
                    onChange={(e) => setPartyCurrency(e.target.value)}
                  />
                </label>
                <label className="space-y-1 text-sm">
                  <span>Email</span>
                  <Input value={partyEmail} onChange={(e) => setPartyEmail(e.target.value)} />
                </label>
                <label className="space-y-1 text-sm">
                  <span>Phone</span>
                  <Input value={partyPhone} onChange={(e) => setPartyPhone(e.target.value)} />
                </label>
                <label className="space-y-1 text-sm">
                  <span>Tax ID</span>
                  <Input value={partyTaxId} onChange={(e) => setPartyTaxId(e.target.value)} />
                </label>
                <label className="space-y-1 text-sm md:col-span-3">
                  <span>Address</span>
                  <Input value={partyAddress} onChange={(e) => setPartyAddress(e.target.value)} />
                </label>
              </div>

              <div className="space-y-2">
                <h3 className="text-sm font-medium">Contacts</h3>
                <p className="text-xs text-muted-foreground">
                  People to reach about a specific matter. Each needs a name and an email or phone.
                </p>
                {partyContacts.map((c, i) => (
                  <div key={i} className="grid grid-cols-12 items-center gap-2">
                    <Input
                      className="col-span-3"
                      placeholder="Name"
                      value={c.name}
                      onChange={(e) => updateContact(i, { name: e.target.value })}
                    />
                    <Input
                      className="col-span-2"
                      placeholder="Designation"
                      value={c.designation}
                      onChange={(e) => updateContact(i, { designation: e.target.value })}
                    />
                    <Input
                      className="col-span-3"
                      placeholder="Email"
                      value={c.email}
                      onChange={(e) => updateContact(i, { email: e.target.value })}
                    />
                    <Input
                      className="col-span-2"
                      placeholder="Phone"
                      value={c.phone}
                      onChange={(e) => updateContact(i, { phone: e.target.value })}
                    />
                    <label className="col-span-1 flex items-center gap-1 text-xs">
                      <input
                        type="radio"
                        name="primary-contact"
                        checked={c.is_primary}
                        onChange={() => setPrimaryContact(i)}
                      />
                      Primary
                    </label>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="col-span-1"
                      onClick={() =>
                        setPartyContacts((prev) => prev.filter((_, idx) => idx !== i))
                      }
                    >
                      Remove
                    </Button>
                  </div>
                ))}
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPartyContacts((prev) => [...prev, { ...BLANK_CONTACT }])}
                >
                  Add contact
                </Button>
              </div>
              <div className="flex items-center justify-between gap-3">
                {partyError ? <p className="text-sm text-red-700">{partyError}</p> : <span />}
                <Button
                  disabled={partyMutation.isPending}
                  onClick={() => partyMutation.mutate()}
                >
                  {partyMutation.isPending ? "Saving…" : "Save"}
                </Button>
              </div>
            </section>
          )}

          {!showPartyForm && partyError && <p className="text-sm text-red-700">{partyError}</p>}

          {partyView === "rolodex" ? (
            partiesQuery.error ? (
              <p className="text-sm text-red-700">
                Failed to load customers and vendors: {partiesQuery.error.message}
              </p>
            ) : partiesQuery.isLoading ? (
              <p className="py-8 text-center text-sm text-muted-foreground">Loading…</p>
            ) : (
              <PartyRolodex parties={partiesQuery.data?.parties ?? []} />
            )
          ) : (
            <DataTable<Record<string, unknown>>
              columns={partyColumns}
              data={partyRows}
              isLoading={partiesQuery.isLoading}
              emptyMessage={
                partiesQuery.error
                  ? `Failed to load customers and vendors: ${partiesQuery.error.message}`
                  : "No customers or vendors yet."
              }
            />
          )}
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

        <TabsContent value="cost-behavior" className="pt-4">
          <CostBehaviorReport />
        </TabsContent>
      </Tabs>
    </div>
  );
}
