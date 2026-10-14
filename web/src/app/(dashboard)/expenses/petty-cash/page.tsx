"use client";

import Link from "next/link";
import { ArrowLeft, Plus, Wallet } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { AmountDisplay } from "@/components/ui/amount-display";
import { StatusBadge } from "@/components/ui/status-badge";
import type { PettyCashAdvance as ApiAdvance } from "@/lib/api/expenses";
import { usePettyCash } from "@/lib/hooks/use-expenses";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
interface PettyCashAdvance {
  id: string;
  issuedTo: string;
  amount: string;
  currency: string;
  purpose: string;
  issueDate: string;
  status: "active" | "settled";
  settledDate: string | null;
  settledAmount: string | null;
  unspentAmount: string | null;
  settlementNotes: string | null;
}

// ---------------------------------------------------------------------------
// API → row mapping
// ---------------------------------------------------------------------------

// The API does not carry a currency on advances; this tenant books in USD.
const CURRENCY = "USD";

/** "1500.00" → cents, for exact subtraction (never float). */
function toCents(amount: string): number {
  const [whole = "0", frac = ""] = amount.split(".");
  return Number(whole) * 100 + Number((frac + "00").slice(0, 2));
}

function fromCents(cents: number): string {
  return `${Math.floor(cents / 100)}.${String(cents % 100).padStart(2, "0")}`;
}

function toAdvance(a: ApiAdvance): PettyCashAdvance {
  const settled = a.status === "settled";
  const unspent = a.unspent_amount || null;
  return {
    id: a.id,
    issuedTo: a.issued_to,
    amount: a.amount,
    currency: CURRENCY,
    purpose: a.purpose,
    issueDate: (a.issued_at ?? "").slice(0, 10),
    status: settled ? "settled" : "active",
    settledDate: a.settled_at ? a.settled_at.slice(0, 10) : null,
    settledAmount: settled && unspent !== null ? fromCents(toCents(a.amount) - toCents(unspent)) : null,
    unspentAmount: unspent,
    settlementNotes: null,
  };
}

export default function PettyCashPage() {
  const advancesQuery = usePettyCash();
  const advances = (advancesQuery.data?.advances ?? []).map(toAdvance);

  const activeAdvances = advances.filter((a) => a.status === "active");
  const settledAdvances = advances.filter((a) => a.status === "settled");

  return (
    <div className="mx-auto max-w-6xl">
      {/* Back link */}
      <Link
        href="/expenses"
        className="mb-4 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Expenses
      </Link>

      <PageHeader
        title="Petty Cash"
        description="Manage cash advances and settlements for small production expenses"
        icon="Wallet"
        actions={
          <button
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Plus className="h-4 w-4" />
            Issue Advance
          </button>
        }
      />

      {/* ── Active Advances ────────────────────────────────────────────── */}
      <section className="mb-8">
        <h2
          className="mb-4 flex items-center gap-2 font-heading text-sm font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          <Wallet className="h-4 w-4" style={{ color: "var(--thittam-primary, #3b82f6)" }} />
          Active Advances ({activeAdvances.length})
        </h2>

        {activeAdvances.length === 0 ? (
          <p
            className="rounded-lg border py-8 text-center text-sm font-body"
            style={{
              borderColor: "var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-muted-foreground, #64748b)",
            }}
          >
            No active advances.
          </p>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {activeAdvances.map((adv) => (
              <div
                key={adv.id}
                className="rounded-xl p-5"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                }}
              >
                <div className="mb-3 flex items-center justify-between">
                  <span
                    className="font-heading text-sm font-semibold"
                    style={{ color: "var(--thittam-foreground, #0f172a)" }}
                  >
                    {adv.issuedTo}
                  </span>
                  <StatusBadge status="active" />
                </div>

                <div className="mb-3">
                  <AmountDisplay amount={adv.amount} currency={adv.currency} size="lg" />
                </div>

                <p
                  className="mb-3 text-sm font-body"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {adv.purpose}
                </p>

                <div className="flex items-center gap-1">
                  <span
                    className="text-xs font-heading"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Issued:
                  </span>
                  <span
                    className="font-mono tabular-nums text-xs"
                    style={{ color: "var(--thittam-foreground, #0f172a)" }}
                  >
                    {adv.issueDate}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* ── Settled ────────────────────────────────────────────────────── */}
      <section>
        <h2
          className="mb-4 flex items-center gap-2 font-heading text-sm font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          <Wallet className="h-4 w-4" style={{ color: "var(--thittam-muted-foreground, #64748b)" }} />
          Settled ({settledAdvances.length})
        </h2>

        {settledAdvances.length === 0 ? (
          <p
            className="rounded-lg border py-8 text-center text-sm font-body"
            style={{
              borderColor: "var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-muted-foreground, #64748b)",
            }}
          >
            No settled advances.
          </p>
        ) : (
          <div
            className="overflow-x-auto rounded-lg border"
            style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
          >
            <table className="w-full text-sm">
              <thead>
                <tr
                  style={{
                    backgroundColor: "var(--thittam-muted, #f1f5f9)",
                    borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                  }}
                >
                  <th
                    className="px-4 py-3 text-left text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Issued To
                  </th>
                  <th
                    className="px-4 py-3 text-left text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Purpose
                  </th>
                  <th
                    className="px-4 py-3 text-right text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Advanced
                  </th>
                  <th
                    className="px-4 py-3 text-right text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Spent
                  </th>
                  <th
                    className="px-4 py-3 text-right text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Unspent
                  </th>
                  <th
                    className="px-4 py-3 text-left text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Settled Date
                  </th>
                  <th
                    className="px-4 py-3 text-left text-xs font-heading font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Notes
                  </th>
                </tr>
              </thead>
              <tbody>
                {settledAdvances.map((adv, idx) => (
                  <tr
                    key={adv.id}
                    className={idx % 2 === 1 ? "bg-gray-50/50" : ""}
                    style={{
                      borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                    }}
                  >
                    <td
                      className="px-4 py-3 font-heading text-sm font-medium"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {adv.issuedTo}
                    </td>
                    <td
                      className="px-4 py-3 font-body text-sm"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {adv.purpose}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <AmountDisplay amount={adv.amount} currency={adv.currency} size="sm" />
                    </td>
                    <td className="px-4 py-3 text-right">
                      <AmountDisplay
                        amount={adv.settledAmount ?? "0.00"}
                        currency={adv.currency}
                        size="sm"
                      />
                    </td>
                    <td className="px-4 py-3 text-right">
                      <span
                        className="font-mono tabular-nums text-sm"
                        style={{ color: "var(--thittam-status-on-track, #16a34a)" }}
                      >
                        <AmountDisplay
                          amount={adv.unspentAmount ?? "0.00"}
                          currency={adv.currency}
                          size="sm"
                        />
                      </span>
                    </td>
                    <td
                      className="px-4 py-3 font-mono tabular-nums text-sm"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {adv.settledDate}
                    </td>
                    <td
                      className="max-w-xs px-4 py-3 font-body text-xs"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      {adv.settlementNotes}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
