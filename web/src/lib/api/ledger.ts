import { api } from "./client";

// ---------------------------------------------------------------------------
// Types — mirror proto/thittam/ledger/v1/ledger.proto (snake_case JSON).
// Monetary amounts are decimal strings ("1500.00"), never numbers.
// ---------------------------------------------------------------------------

export interface LedgerAccount {
  id: string;
  tenant_id: string;
  code: string;
  name: string;
  account_type: string; // asset|liability|equity|revenue|expense
  parent_id: string;
  is_active: boolean;
  // Bank accounts only; empty strings for all other accounts.
  bank_name: string;
  account_last4: string;
  bank_currency: string;
}

export interface AccountingPeriod {
  id: string;
  tenant_id: string;
  year: number;
  month: number;
  status: string; // open|closed
  closed_by: string;
  closed_at: string | null;
}

export interface JournalLine {
  id: string;
  journal_id: string;
  account_id: string;
  debit_amount: string;
  credit_amount: string;
  currency: string;
  description: string;
  party_id?: string; // empty or absent when the line has no party
}

export interface JournalEntry {
  id: string;
  tenant_id: string;
  production_id: string;
  period_id: string;
  entry_number: string;
  reference: string;
  narration: string;
  status: string; // draft|posted|void
  lines: JournalLine[];
  posted_by: string;
  posted_at: string | null;
  created_at: string | null;
}

export interface TrialBalanceEntry {
  account_id: string;
  code: string;
  name: string;
  account_type: string;
  debit: string;
  credit: string;
  balance: string; // debit − credit; negative means credit balance
}

export interface JournalLineInput {
  account_id: string;
  debit_amount: string;
  credit_amount: string;
  currency: string;
  description: string;
  party_id?: string;
}

export interface CreateJournalEntryInput {
  period_id: string;
  reference?: string;
  narration: string;
  lines: JournalLineInput[];
}

// ---------------------------------------------------------------------------
// Money helpers — exact integer cents; never float arithmetic on amounts.
// ---------------------------------------------------------------------------

/** "1500.00" → 150000. Returns 0 for empty or malformed input. */
export function toCents(amount: string): number {
  const m = /^(-?)(\d+)(?:\.(\d{1,2}))?$/.exec(amount.trim());
  if (!m) return 0;
  const whole = Number(m[2]) * 100;
  const frac = Number((m[3] ?? "").padEnd(2, "0"));
  return m[1] === "-" ? -(whole + frac) : whole + frac;
}

/** 150000 → "1500.00". */
export function fromCents(cents: number): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  const whole = Math.floor(abs / 100);
  const frac = String(abs % 100).padStart(2, "0");
  return `${sign}${whole}.${frac}`;
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

const BASE = "/api/v1/ledger";

export async function listLedgerAccounts(): Promise<{ accounts: LedgerAccount[] }> {
  return api.getList<{ accounts: LedgerAccount[] }>(`${BASE}/accounts`);
}

export interface CreateBankAccountInput {
  bank_name: string;
  account_number: string; // only the last four digits are kept
  bank_currency: string; // ISO 4217, e.g. USD
}

// The service assigns the account code and returns the account with only the
// last four digits of the number.
export async function createBankAccount(
  data: CreateBankAccountInput,
): Promise<LedgerAccount> {
  return api.post<LedgerAccount>(`${BASE}/accounts`, data);
}

export async function listAccountingPeriods(): Promise<{ periods: AccountingPeriod[] }> {
  return api.getList<{ periods: AccountingPeriod[] }>(`${BASE}/periods`);
}

export async function listJournalEntries(params?: {
  period_id?: string;
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<{ entries: JournalEntry[] }> {
  const q = new URLSearchParams();
  if (params?.period_id) q.set("period_id", params.period_id);
  if (params?.status) q.set("status", params.status);
  if (params?.limit) q.set("limit", String(params.limit));
  if (params?.offset) q.set("offset", String(params.offset));
  const qs = q.toString() ? `?${q}` : "";
  return api.getList<{ entries: JournalEntry[] }>(`${BASE}/journal-entries${qs}`);
}

export async function createJournalEntry(
  data: CreateJournalEntryInput,
): Promise<JournalEntry> {
  return api.post<JournalEntry>(`${BASE}/journal-entries`, data);
}

export async function postJournalEntry(id: string): Promise<JournalEntry> {
  return api.post<JournalEntry>(`${BASE}/journal-entries/${id}/post`, {});
}

// ---------------------------------------------------------------------------
// Parties (customers and vendors)
// ---------------------------------------------------------------------------

export interface Party {
  id: string;
  tenant_id: string;
  name: string;
  party_type: string; // customer|supplier|both
  email: string;
  phone: string;
  tax_id: string;
  address: string;
  default_currency: string; // ISO 4217 or empty
  is_active: boolean;
  contacts: PartyContact[];
}

// A person at a party. The organization-level email and phone live on Party.
export interface PartyContact {
  id: string;
  party_id: string;
  name: string;
  designation: string; // free text, e.g. "Accounts payable"
  email: string;
  phone: string;
  is_primary: boolean;
}

export type PartyContactInput = Omit<PartyContact, "id" | "party_id">;

export interface PartyInput {
  name: string;
  party_type: string;
  email: string;
  phone: string;
  tax_id: string;
  address: string;
  default_currency: string;
  contacts: PartyContactInput[];
}

// party_type "customer" returns customers and both; "supplier" returns suppliers and both.
export async function listParties(params?: {
  party_type?: string;
  include_inactive?: boolean;
}): Promise<{ parties: Party[] }> {
  const q = new URLSearchParams();
  if (params?.party_type) q.set("party_type", params.party_type);
  if (params?.include_inactive) q.set("include_inactive", "true");
  const qs = q.toString() ? `?${q}` : "";
  return api.getList<{ parties: Party[] }>(`${BASE}/parties${qs}`);
}

export async function createParty(data: PartyInput): Promise<Party> {
  return api.post<Party>(`${BASE}/parties`, data);
}

// Replaces every editable field; send the full record.
export async function updateParty(
  id: string,
  data: PartyInput & { is_active: boolean },
): Promise<Party> {
  return api.put<Party>(`${BASE}/parties/${id}`, data);
}

export async function getTrialBalance(): Promise<{ entries: TrialBalanceEntry[] }> {
  return api.getList<{ entries: TrialBalanceEntry[] }>(`${BASE}/trial-balance`);
}

// ---------------------------------------------------------------------------
// Party transactions (view only)
// ---------------------------------------------------------------------------

export interface PartyLedgerLine {
  entry_id: string;
  entry_number: string;
  entry_date: string; // RFC 3339
  narration: string;
  account_code: string;
  account_name: string;
  debit_amount: string;
  credit_amount: string;
  currency: string;
  description: string;
}

export interface PartyTransactions {
  lines: PartyLedgerLine[];
  total_debit: string;
  total_credit: string;
}

export async function listPartyTransactions(partyId: string): Promise<PartyTransactions> {
  return api.getList<PartyTransactions>(`${BASE}/parties/${partyId}/transactions`);
}
