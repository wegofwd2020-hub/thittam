"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  CreditCard,
  Download,
  Star,
  Trash2,
  CheckCircle2,
  AlertTriangle,
  Clock,
} from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { StatusBadge } from "@/components/ui/status-badge";
import { AmountDisplay } from "@/components/ui/amount-display";
import { PlanBadge } from "@/components/platform/plan-badge";
import { DataTable, type Column } from "@/components/ui/data-table";

// ---------------------------------------------------------------------------
// Types (mirrors services/billing/models.go)
// ---------------------------------------------------------------------------

type SubscriptionStatus = "trialing" | "active" | "past_due" | "cancelled" | "suspended";
type BillingCycle = "monthly" | "annual";
type InvoiceStatus = "pending" | "paid" | "overdue" | "void";
type PaymentMethodType = "card" | "upi" | "netbanking" | "bank_transfer";

interface Subscription {
  id: string;
  plan: string;
  status: SubscriptionStatus;
  billing_cycle: BillingCycle;
  current_period_start: string;
  current_period_end: string;
  trial_ends_at: string | null;
  cancelled_at: string | null;
}

interface Invoice {
  id: string;
  invoice_number: string;
  period_start: string;
  period_end: string;
  plan: string;
  amount: string;
  tax_amount: string;
  total_amount: string;
  currency: string;
  status: InvoiceStatus;
  due_date: string;
  paid_at: string | null;
  payment_method: string | null;
}

interface PaymentMethod {
  id: string;
  type: PaymentMethodType;
  display_name: string;
  is_default: boolean;
  expires_at: string | null;
}

interface UsageSummary {
  plan: string;
  active_productions: number;
  max_productions: number; // -1 = unlimited
  user_count: number;
  max_users: number;
  storage_gb: string;
  max_storage_gb: number;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// Mock data — XYZ CBA Productions on Professional plan
// ---------------------------------------------------------------------------

const mockSubscription: Subscription = {
  id: "sub-001",
  plan: "professional",
  status: "active",
  billing_cycle: "monthly",
  current_period_start: "2026-04-01T00:00:00Z",
  current_period_end: "2026-04-30T23:59:59Z",
  trial_ends_at: null,
  cancelled_at: null,
};

const mockUsage: UsageSummary = {
  plan: "professional",
  active_productions: 2,
  max_productions: 10,
  user_count: 6,
  max_users: 50,
  storage_gb: "12.40",
  max_storage_gb: 50,
  updated_at: "2026-04-10T03:00:00Z",
};

const mockInvoices: Invoice[] = [
  {
    id: "inv-001",
    invoice_number: "INV-2026-00003",
    period_start: "2026-04-01T00:00:00Z",
    period_end: "2026-04-30T23:59:59Z",
    plan: "professional",
    amount: "14999.00",
    tax_amount: "2699.82",
    total_amount: "17698.82",
    currency: "INR",
    status: "pending",
    due_date: "2026-04-10T00:00:00Z",
    paid_at: null,
    payment_method: null,
  },
  {
    id: "inv-002",
    invoice_number: "INV-2026-00002",
    period_start: "2026-03-01T00:00:00Z",
    period_end: "2026-03-31T23:59:59Z",
    plan: "professional",
    amount: "14999.00",
    tax_amount: "2699.82",
    total_amount: "17698.82",
    currency: "INR",
    status: "paid",
    due_date: "2026-03-10T00:00:00Z",
    paid_at: "2026-03-08T11:34:00Z",
    payment_method: "Visa ending 4242",
  },
  {
    id: "inv-003",
    invoice_number: "INV-2026-00001",
    period_start: "2026-02-01T00:00:00Z",
    period_end: "2026-02-28T23:59:59Z",
    plan: "professional",
    amount: "14999.00",
    tax_amount: "2699.82",
    total_amount: "17698.82",
    currency: "INR",
    status: "paid",
    due_date: "2026-02-10T00:00:00Z",
    paid_at: "2026-02-07T09:12:00Z",
    payment_method: "Visa ending 4242",
  },
];

const mockPaymentMethods: PaymentMethod[] = [
  {
    id: "pm-001",
    type: "card",
    display_name: "Visa ending 4242",
    is_default: true,
    expires_at: "2028-12-01T00:00:00Z",
  },
  {
    id: "pm-002",
    type: "upi",
    display_name: "rajesh.kumar@okaxis",
    is_default: false,
    expires_at: null,
  },
];

// Plan comparison table data
const PLAN_FEATURES = [
  {
    feature: "Active productions",
    starter: "2",
    professional: "10",
    enterprise: "Unlimited",
  },
  { feature: "Team members", starter: "10", professional: "50", enterprise: "Unlimited" },
  { feature: "Storage", starter: "5 GB", professional: "50 GB", enterprise: "Unlimited" },
  { feature: "Budget versions", starter: "3 per production", professional: "Unlimited", enterprise: "Unlimited" },
  { feature: "Expense approvals", starter: "Single-level", professional: "Dual-level", enterprise: "Custom" },
  { feature: "Reports", starter: "Standard", professional: "Advanced", enterprise: "Custom + API" },
  { feature: "SSO / OIDC", starter: "\u2014", professional: "\u2713", enterprise: "\u2713" },
  { feature: "Priority support", starter: "\u2014", professional: "Email", enterprise: "Dedicated CSM" },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
  });
}

function formatDateTime(iso: string | null): string {
  if (!iso) return "\u2014";
  return new Date(iso).toLocaleString("en-IN", {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function billingCycleLabel(cycle: BillingCycle): string {
  return cycle === "annual" ? "Annual" : "Monthly";
}

function daysUntil(iso: string): number {
  const now = new Date();
  const target = new Date(iso);
  return Math.ceil((target.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function UsageGauge({
  label,
  used,
  max,
  unit,
}: {
  label: string;
  used: number | string;
  max: number;
  unit?: string;
}) {
  const usedNum = typeof used === "string" ? parseFloat(used) : used;
  const pct = max === -1 ? 0 : Math.min(100, (usedNum / max) * 100);
  const isUnlimited = max === -1;
  const isWarning = !isUnlimited && pct >= 80;
  const isCritical = !isUnlimited && pct >= 95;

  const barColor = isCritical
    ? "#ef4444"
    : isWarning
      ? "#f59e0b"
      : "var(--thittam-primary, #3b82f6)";

  return (
    <div
      className="rounded-xl p-5"
      style={{
        backgroundColor: "var(--thittam-background, #fff)",
        border: "1px solid var(--thittam-border, #e2e8f0)",
      }}
    >
      <span
        className="block text-xs font-heading font-medium uppercase tracking-wider"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        {label}
      </span>
      <div className="mt-2 flex items-end justify-between gap-2">
        <span
          className="font-mono tabular-nums text-2xl font-semibold"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {typeof used === "string" ? used : used.toLocaleString()}
          {unit && (
            <span className="ml-1 text-sm font-normal" style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>
              {unit}
            </span>
          )}
        </span>
        <span
          className="font-mono tabular-nums text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {isUnlimited ? "/ \u221e" : `/ ${max.toLocaleString()}${unit ? ` ${unit}` : ""}`}
        </span>
      </div>
      {!isUnlimited && (
        <div className="mt-3">
          <div
            className="h-1.5 w-full overflow-hidden rounded-full"
            style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
          >
            <div
              className="h-full rounded-full transition-all duration-500"
              style={{ width: `${pct}%`, backgroundColor: barColor }}
            />
          </div>
          {isWarning && (
            <p
              className="mt-1.5 flex items-center gap-1 text-[11px] font-body"
              style={{ color: isWarning ? "#92400e" : "#64748b" }}
            >
              <AlertTriangle className="h-3 w-3 shrink-0" />
              {isCritical ? "At limit — upgrade to add more" : "Approaching limit"}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function PaymentMethodIcon({ type }: { type: PaymentMethodType }) {
  const labels: Record<PaymentMethodType, string> = {
    card: "CARD",
    upi: "UPI",
    netbanking: "NET",
    bank_transfer: "NEFT",
  };
  const colors: Record<PaymentMethodType, { bg: string; text: string }> = {
    card: { bg: "#dbeafe", text: "#1d4ed8" },
    upi: { bg: "#d1fae5", text: "#065f46" },
    netbanking: { bg: "#fef3c7", text: "#92400e" },
    bank_transfer: { bg: "#f3e8ff", text: "#6b21a8" },
  };
  const c = colors[type];
  return (
    <span
      className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-bold font-heading"
      style={{ backgroundColor: c.bg, color: c.text }}
    >
      {labels[type]}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Tab definitions
// ---------------------------------------------------------------------------

const TABS = [
  { key: "overview", label: "Overview" },
  { key: "invoices", label: "Invoices" },
  { key: "payment-methods", label: "Payment Methods" },
] as const;

type TabKey = (typeof TABS)[number]["key"];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function BillingPage() {
  const [activeTab, setActiveTab] = useState<TabKey>("overview");
  const [paymentMethods, setPaymentMethods] = useState(mockPaymentMethods);

  const sub = mockSubscription;
  const usage = mockUsage;
  const daysLeft = daysUntil(sub.current_period_end);

  // Invoice table columns
  const invoiceColumns: Column<Invoice>[] = [
    {
      key: "invoice_number",
      header: "Invoice",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-sm font-medium"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {row.invoice_number}
        </span>
      ),
    },
    {
      key: "period_start",
      header: "Period",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-foreground, #0f172a)" }}
        >
          {formatDate(row.period_start)} – {formatDate(row.period_end)}
        </span>
      ),
    },
    {
      key: "plan",
      header: "Plan",
      render: (row) => <PlanBadge plan={row.plan} />,
    },
    {
      key: "amount",
      header: "Subtotal",
      type: "number",
      render: (row) => <AmountDisplay amount={row.amount} currency={row.currency} size="sm" />,
    },
    {
      key: "tax_amount",
      header: "GST 18%",
      type: "number",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          <AmountDisplay amount={row.tax_amount} currency={row.currency} size="sm" />
        </span>
      ),
    },
    {
      key: "total_amount",
      header: "Total",
      type: "number",
      render: (row) => <AmountDisplay amount={row.total_amount} currency={row.currency} size="sm" />,
    },
    {
      key: "status",
      header: "Status",
      type: "status",
      render: (row) => <StatusBadge status={row.status} />,
    },
    {
      key: "paid_at",
      header: "Paid On",
      type: "date",
      render: (row) => (
        <span
          className="font-mono tabular-nums text-xs"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {formatDateTime(row.paid_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <button
          onClick={(e) => {
            e.stopPropagation();
            toast.success(`Downloading ${row.invoice_number}.pdf`);
          }}
          className="inline-flex items-center gap-1 text-xs font-heading font-medium transition-opacity hover:opacity-70"
          style={{ color: "var(--thittam-primary, #3b82f6)" }}
          title="Download PDF"
        >
          <Download className="h-3.5 w-3.5" />
          PDF
        </button>
      ),
    },
  ];

  function handleSetDefault(id: string) {
    setPaymentMethods((prev) =>
      prev.map((pm) => ({ ...pm, is_default: pm.id === id }))
    );
    toast.success("Default payment method updated");
  }

  function handleRemove(id: string) {
    const pm = paymentMethods.find((p) => p.id === id);
    if (pm?.is_default) {
      toast.error("Cannot remove the default payment method. Set another as default first.");
      return;
    }
    if (!window.confirm(`Remove "${pm?.display_name}"?`)) return;
    setPaymentMethods((prev) => prev.filter((p) => p.id !== id));
    toast.success("Payment method removed");
  }

  return (
    <div className="mx-auto max-w-7xl">
      <PageHeader
        title="Billing"
        description="Manage your subscription, invoices, and payment methods"
        icon="CreditCard"
      />

      {/* Tab navigation */}
      <div
        className="mb-6 flex border-b"
        style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
      >
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2.5 text-sm font-heading font-medium transition-colors ${
              activeTab === tab.key ? "border-b-2" : ""
            }`}
            style={
              activeTab === tab.key
                ? {
                    borderColor: "var(--thittam-primary, #3b82f6)",
                    color: "var(--thittam-primary, #3b82f6)",
                  }
                : { color: "var(--thittam-muted-foreground, #64748b)" }
            }
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* ================================================================= */}
      {/* Overview Tab                                                      */}
      {/* ================================================================= */}
      {activeTab === "overview" && (
        <div className="space-y-6">
          {/* Subscription status card */}
          <div
            className="rounded-xl p-6"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
              {/* Left — plan + status */}
              <div className="flex items-start gap-4">
                <div
                  className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl"
                  style={{
                    backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 12%, transparent)",
                    color: "var(--thittam-primary, #3b82f6)",
                  }}
                >
                  <CreditCard className="h-6 w-6" />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <PlanBadge plan={sub.plan} />
                    <StatusBadge status={sub.status} />
                  </div>
                  <p
                    className="mt-2 text-sm font-body"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    {billingCycleLabel(sub.billing_cycle)} billing &middot; current period{" "}
                    <span className="font-mono tabular-nums">
                      {formatDate(sub.current_period_start)}
                    </span>{" "}
                    &ndash;{" "}
                    <span className="font-mono tabular-nums">
                      {formatDate(sub.current_period_end)}
                    </span>
                  </p>
                  {sub.trial_ends_at && (
                    <p
                      className="mt-1 flex items-center gap-1.5 text-xs font-body"
                      style={{ color: "#92400e" }}
                    >
                      <Clock className="h-3.5 w-3.5" />
                      Trial ends {formatDate(sub.trial_ends_at)}
                    </p>
                  )}
                  {sub.status === "active" && daysLeft <= 7 && (
                    <p
                      className="mt-1 flex items-center gap-1.5 text-xs font-body"
                      style={{ color: "#92400e" }}
                    >
                      <Clock className="h-3.5 w-3.5" />
                      Renews in {daysLeft} {daysLeft === 1 ? "day" : "days"}
                    </p>
                  )}
                  {sub.cancelled_at && (
                    <p
                      className="mt-1 flex items-center gap-1.5 text-xs font-body text-red-600"
                    >
                      <AlertTriangle className="h-3.5 w-3.5" />
                      Cancelled on {formatDate(sub.cancelled_at)} — access until period end
                    </p>
                  )}
                </div>
              </div>

              {/* Right — upgrade / cancel actions */}
              {sub.status !== "cancelled" && (
                <div className="flex shrink-0 flex-col gap-2 sm:items-end">
                  {sub.plan !== "enterprise" && (
                    <button
                      onClick={() => toast.success("Upgrade flow — contact sales@thittam.io")}
                      className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-opacity hover:opacity-90"
                      style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
                    >
                      Upgrade Plan
                    </button>
                  )}
                  <button
                    onClick={() => {
                      if (
                        window.confirm(
                          "Cancel subscription? You will retain access until the end of the current billing period."
                        )
                      ) {
                        toast.success("Cancellation requested — effective end of period");
                      }
                    }}
                    className="text-xs font-heading font-medium transition-opacity hover:opacity-70 text-red-500"
                  >
                    Cancel subscription
                  </button>
                </div>
              )}
            </div>
          </div>

          {/* Usage gauges */}
          <div>
            <h2
              className="mb-3 font-heading text-base font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Usage this period
            </h2>
            <div className="grid gap-4 sm:grid-cols-3">
              <UsageGauge
                label="Active Productions"
                used={usage.active_productions}
                max={usage.max_productions}
              />
              <UsageGauge
                label="Team Members"
                used={usage.user_count}
                max={usage.max_users}
              />
              <UsageGauge
                label="Storage"
                used={usage.storage_gb}
                max={usage.max_storage_gb}
                unit="GB"
              />
            </div>
            <p
              className="mt-2 text-xs font-body"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            >
              Usage data updated nightly. Last updated:{" "}
              <span className="font-mono tabular-nums">{formatDateTime(usage.updated_at)}</span>
            </p>
          </div>

          {/* Plan comparison */}
          <div>
            <h2
              className="mb-3 font-heading text-base font-semibold"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Plan comparison
            </h2>
            <div
              className="overflow-x-auto rounded-xl border"
              style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
            >
              <table className="w-full text-sm">
                <thead>
                  <tr style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}>
                    <th
                      className="px-4 py-3 text-left text-xs font-semibold font-heading uppercase tracking-wider"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      Feature
                    </th>
                    {["starter", "professional", "enterprise"].map((plan) => (
                      <th
                        key={plan}
                        className="px-4 py-3 text-center text-xs font-semibold font-heading uppercase tracking-wider"
                        style={{
                          color:
                            plan === sub.plan
                              ? "var(--thittam-primary, #3b82f6)"
                              : "var(--thittam-muted-foreground, #64748b)",
                          borderBottom:
                            plan === sub.plan
                              ? "2px solid var(--thittam-primary, #3b82f6)"
                              : undefined,
                        }}
                      >
                        {plan.charAt(0).toUpperCase() + plan.slice(1)}
                        {plan === sub.plan && (
                          <span className="ml-1 text-[10px] normal-case">(current)</span>
                        )}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {PLAN_FEATURES.map((row, idx) => (
                    <tr
                      key={row.feature}
                      style={{
                        borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                        backgroundColor: idx % 2 === 1 ? "rgba(241,245,249,0.4)" : undefined,
                      }}
                    >
                      <td
                        className="px-4 py-3 font-body text-sm"
                        style={{ color: "var(--thittam-foreground, #0f172a)" }}
                      >
                        {row.feature}
                      </td>
                      {(["starter", "professional", "enterprise"] as const).map((plan) => {
                        const val = row[plan];
                        const isCurrent = plan === sub.plan;
                        return (
                          <td
                            key={plan}
                            className="px-4 py-3 text-center font-body text-sm"
                            style={{
                              color:
                                val === "\u2014"
                                  ? "var(--thittam-muted-foreground, #64748b)"
                                  : isCurrent
                                    ? "var(--thittam-foreground, #0f172a)"
                                    : "var(--thittam-foreground, #0f172a)",
                              fontWeight: isCurrent ? 500 : undefined,
                            }}
                          >
                            {val === "\u2713" ? (
                              <CheckCircle2 className="mx-auto h-4 w-4 text-emerald-500" />
                            ) : val === "\u2014" ? (
                              <span style={{ color: "var(--thittam-muted-foreground, #64748b)" }}>—</span>
                            ) : (
                              val
                            )}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* ================================================================= */}
      {/* Invoices Tab                                                      */}
      {/* ================================================================= */}
      {activeTab === "invoices" && (
        <div>
          <div className="mb-4 flex items-center justify-between">
            <div>
              <h2
                className="font-heading text-lg font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Invoices
              </h2>
              <p
                className="mt-0.5 text-sm font-body"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                All amounts include 18% GST (SAC 998314). PDFs are available for
                download for your records.
              </p>
            </div>
          </div>

          <DataTable<Record<string, unknown>>
            columns={invoiceColumns as unknown as Column<Record<string, unknown>>[]}
            data={mockInvoices as unknown as Record<string, unknown>[]}
            emptyMessage="No invoices yet."
          />
        </div>
      )}

      {/* ================================================================= */}
      {/* Payment Methods Tab                                               */}
      {/* ================================================================= */}
      {activeTab === "payment-methods" && (
        <div>
          <div className="mb-4 flex items-center justify-between">
            <div>
              <h2
                className="font-heading text-lg font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Payment Methods
              </h2>
              <p
                className="mt-0.5 text-sm font-body"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Stored payment instruments for automatic billing. Raw card
                numbers are never stored — only gateway tokens.
              </p>
            </div>
            <button
              onClick={() => toast.success("Add payment method — Razorpay checkout will open")}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-opacity hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              Add Payment Method
            </button>
          </div>

          {paymentMethods.length === 0 ? (
            <div
              className="flex items-center justify-center rounded-lg border py-16 font-body text-sm"
              style={{
                borderColor: "var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-muted-foreground, #64748b)",
              }}
            >
              No payment methods saved. Add one to enable automatic billing.
            </div>
          ) : (
            <ul className="space-y-3">
              {paymentMethods.map((pm) => (
                <li
                  key={pm.id}
                  className="flex items-center justify-between gap-4 rounded-xl px-5 py-4"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: pm.is_default
                      ? "1.5px solid var(--thittam-primary, #3b82f6)"
                      : "1px solid var(--thittam-border, #e2e8f0)",
                  }}
                >
                  {/* Left — icon + name + default badge */}
                  <div className="flex items-center gap-3">
                    <PaymentMethodIcon type={pm.type} />
                    <div>
                      <div className="flex items-center gap-2">
                        <span
                          className="font-heading text-sm font-medium"
                          style={{ color: "var(--thittam-foreground, #0f172a)" }}
                        >
                          {pm.display_name}
                        </span>
                        {pm.is_default && (
                          <span
                            className="inline-flex items-center gap-0.5 rounded-full px-2 py-0.5 text-[10px] font-medium font-heading"
                            style={{
                              backgroundColor:
                                "color-mix(in srgb, var(--thittam-primary, #3b82f6) 12%, transparent)",
                              color: "var(--thittam-primary, #3b82f6)",
                            }}
                          >
                            <Star className="h-2.5 w-2.5 fill-current" />
                            Default
                          </span>
                        )}
                      </div>
                      {pm.expires_at && (
                        <span
                          className="font-mono tabular-nums text-xs"
                          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                        >
                          Expires {formatDate(pm.expires_at)}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Right — actions */}
                  <div className="flex items-center gap-3">
                    {!pm.is_default && (
                      <button
                        onClick={() => handleSetDefault(pm.id)}
                        className="flex items-center gap-1 text-xs font-heading font-medium transition-opacity hover:opacity-70"
                        style={{ color: "var(--thittam-primary, #3b82f6)" }}
                      >
                        <Star className="h-3.5 w-3.5" />
                        Set Default
                      </button>
                    )}
                    <button
                      onClick={() => handleRemove(pm.id)}
                      className="flex items-center gap-1 text-xs font-heading font-medium text-red-500 transition-opacity hover:opacity-70"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      Remove
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}

          {/* Gateway disclosure note */}
          <div
            className="mt-6 rounded-lg px-4 py-3 text-xs font-body"
            style={{
              backgroundColor: "var(--thittam-muted, #f1f5f9)",
              color: "var(--thittam-muted-foreground, #64748b)",
            }}
          >
            Payments are processed via Razorpay (India) or Stripe (international). Thittam
            stores only gateway-issued tokens — raw card numbers are never transmitted to our
            servers. For billing disputes, contact{" "}
            <a
              href="mailto:billing@thittam.io"
              className="font-medium underline"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            >
              billing@thittam.io
            </a>
            .
          </div>
        </div>
      )}
    </div>
  );
}
