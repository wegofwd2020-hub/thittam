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

export async function getTrialBalance(): Promise<{ entries: TrialBalanceEntry[] }> {
  return api.getList<{ entries: TrialBalanceEntry[] }>(`${BASE}/trial-balance`);
}
