"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, FileText, Plus, Check } from "lucide-react";
import { useTheme } from "@/lib/themes/provider";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type CreationPath = "template" | "blank" | null;

interface TemplateLineItem {
  category: string;
  description: string;
  accountCode: string;
  defaultAmount: string;
}

interface BudgetTemplate {
  id: string;
  name: string;
  description: string;
  lineItems: TemplateLineItem[];
}

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------
const mockProductions = [
  { id: "1", title: "The Last Horizon" },
  { id: "2", title: "Midnight Express Reboot" },
  { id: "3", title: "Project Starfall" },
  { id: "4", title: "Urban Legends: Season 2" },
];

const mockTemplates: BudgetTemplate[] = [
  {
    id: "t1",
    name: "Feature Film",
    description:
      "Standard feature film budget with ATL, BTL, and post-production categories pre-configured.",
    lineItems: [
      { category: "Above the Line", description: "Director", accountCode: "1100", defaultAmount: "15000000.00" },
      { category: "Above the Line", description: "Lead Actor", accountCode: "1200", defaultAmount: "20000000.00" },
      { category: "Above the Line", description: "Lead Actress", accountCode: "1201", defaultAmount: "12000000.00" },
      { category: "Above the Line", description: "Screenwriter", accountCode: "1300", defaultAmount: "3000000.00" },
      { category: "Below the Line", description: "Cinematographer", accountCode: "2100", defaultAmount: "5000000.00" },
      { category: "Below the Line", description: "Art Director", accountCode: "2200", defaultAmount: "3500000.00" },
      { category: "Below the Line", description: "Camera Equipment", accountCode: "2300", defaultAmount: "4500000.00" },
      { category: "Below the Line", description: "Locations", accountCode: "2400", defaultAmount: "6000000.00" },
      { category: "Below the Line", description: "Crew", accountCode: "2500", defaultAmount: "8000000.00" },
      { category: "Below the Line", description: "Transport", accountCode: "2600", defaultAmount: "2000000.00" },
      { category: "Post Production", description: "VFX", accountCode: "3100", defaultAmount: "4000000.00" },
      { category: "Post Production", description: "Sound Design", accountCode: "3200", defaultAmount: "1500000.00" },
      { category: "Post Production", description: "Music & Score", accountCode: "3300", defaultAmount: "500000.00" },
    ],
  },
  {
    id: "t2",
    name: "Documentary",
    description: "Lightweight documentary budget template with reduced ATL and emphasis on post.",
    lineItems: [
      { category: "Above the Line", description: "Director / Producer", accountCode: "1100", defaultAmount: "5000000.00" },
      { category: "Below the Line", description: "Camera Crew", accountCode: "2100", defaultAmount: "3000000.00" },
      { category: "Below the Line", description: "Travel & Locations", accountCode: "2400", defaultAmount: "4000000.00" },
      { category: "Post Production", description: "Editing", accountCode: "3100", defaultAmount: "2000000.00" },
      { category: "Post Production", description: "Sound & Music", accountCode: "3200", defaultAmount: "1000000.00" },
    ],
  },
  {
    id: "t3",
    name: "Short Film",
    description: "Compact budget template for short-format productions.",
    lineItems: [
      { category: "Above the Line", description: "Director", accountCode: "1100", defaultAmount: "500000.00" },
      { category: "Below the Line", description: "Cast & Crew", accountCode: "2100", defaultAmount: "1500000.00" },
      { category: "Below the Line", description: "Locations & Equipment", accountCode: "2400", defaultAmount: "1000000.00" },
      { category: "Post Production", description: "Post & VFX", accountCode: "3100", defaultAmount: "500000.00" },
    ],
  },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function formatINR(amount: string): string {
  const num = parseFloat(amount);
  if (isNaN(num)) return amount;
  const [intPart, decPart] = Math.abs(num).toFixed(2).split(".");
  if (intPart.length <= 3) return `\u20B9${intPart}.${decPart}`;
  const last3 = intPart.slice(-3);
  const rest = intPart.slice(0, -3);
  const grouped = rest.replace(/\B(?=(\d{2})+(?!\d))/g, ",");
  return `\u20B9${grouped},${last3}.${decPart}`;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------
export default function NewBudgetPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();

  // Step tracking
  const [step, setStep] = useState(1);
  const [selectedProduction, setSelectedProduction] = useState("");
  const [creationPath, setCreationPath] = useState<CreationPath>(null);
  const [selectedTemplate, setSelectedTemplate] = useState<BudgetTemplate | null>(null);
  const [label, setLabel] = useState("");
  const [currency, setCurrency] = useState("INR");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showToast, setShowToast] = useState(false);

  function handleNextFromProduction() {
    if (!selectedProduction) return;
    setStep(2);
  }

  function handleSelectPath(path: CreationPath) {
    setCreationPath(path);
    if (path === "blank") {
      setSelectedTemplate(null);
      setStep(3);
    }
  }

  function handleSelectTemplate(template: BudgetTemplate) {
    setSelectedTemplate(template);
    setStep(3);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!label.trim()) return;

    setIsSubmitting(true);
    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 600));
    setIsSubmitting(false);
    setShowToast(true);

    // Navigate after brief toast display
    setTimeout(() => {
      router.push("/budgets");
    }, 1200);
  }

  return (
    <div className="mx-auto max-w-2xl">
      {/* Back link */}
      <Link
        href="/budgets"
        className="mb-6 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Budgets
      </Link>

      {/* Page heading */}
      <h1
        className="mb-1 font-heading text-2xl font-semibold tracking-tight"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        Create Budget
      </h1>
      <p
        className="mb-8 font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Set up a new budget version for one of your{" "}
        {entityLabels.projectPlural.toLowerCase()}.
      </p>

      {/* Step indicator */}
      <div className="mb-8 flex items-center gap-2">
        {[1, 2, 3].map((s) => (
          <div key={s} className="flex items-center gap-2">
            <div
              className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-heading font-semibold ${
                step >= s ? "text-white" : ""
              }`}
              style={
                step >= s
                  ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
                  : {
                      backgroundColor: "var(--thittam-muted, #f1f5f9)",
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }
              }
            >
              {step > s ? <Check className="h-3.5 w-3.5" /> : s}
            </div>
            {s < 3 && (
              <div
                className="h-0.5 w-8"
                style={{
                  backgroundColor:
                    step > s
                      ? "var(--thittam-primary, #3b82f6)"
                      : "var(--thittam-border, #e2e8f0)",
                }}
              />
            )}
          </div>
        ))}
        <span
          className="ml-3 text-xs font-heading font-medium"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          {step === 1
            ? `Select ${entityLabels.project}`
            : step === 2
              ? "Choose path"
              : "Budget details"}
        </span>
      </div>

      {/* ── Step 1: Select production ──────────────────────────────────── */}
      {step === 1 && (
        <div
          className="rounded-xl p-6"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <label
            htmlFor="production"
            className="mb-1.5 block text-sm font-heading font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {entityLabels.project} <span className="text-red-500">*</span>
          </label>
          <p
            className="mb-4 text-xs font-body"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Which {entityLabels.project.toLowerCase()} is this budget for?
          </p>
          <select
            id="production"
            value={selectedProduction}
            onChange={(e) => setSelectedProduction(e.target.value)}
            className="w-full rounded-lg px-3 py-2 text-sm font-body"
            style={{
              backgroundColor: "var(--thittam-background, #fff)",
              border: `1px solid ${!selectedProduction ? "var(--thittam-border, #e2e8f0)" : "var(--thittam-primary, #3b82f6)"}`,
              color: "var(--thittam-foreground, #0f172a)",
            }}
          >
            <option value="">Select a {entityLabels.project.toLowerCase()}...</option>
            {mockProductions.map((p) => (
              <option key={p.id} value={p.id}>
                {p.title}
              </option>
            ))}
          </select>

          <div className="mt-6 flex justify-end">
            <button
              type="button"
              disabled={!selectedProduction}
              onClick={handleNextFromProduction}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              Continue
            </button>
          </div>
        </div>
      )}

      {/* ── Step 2: Choose path ────────────────────────────────────────── */}
      {step === 2 && (
        <div className="flex flex-col gap-4">
          {/* Path selection cards */}
          <div className="grid gap-4 sm:grid-cols-2">
            {/* From template */}
            <button
              type="button"
              onClick={() => handleSelectPath("template")}
              className={`rounded-xl p-5 text-left transition-all ${
                creationPath === "template" ? "ring-2" : ""
              }`}
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                ...(creationPath === "template"
                  ? { borderColor: "var(--thittam-primary, #3b82f6)", ringColor: "var(--thittam-primary, #3b82f6)" }
                  : {}),
              }}
            >
              <div
                className="mb-2 flex h-9 w-9 items-center justify-center rounded-lg"
                style={{
                  backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 12%, transparent)",
                  color: "var(--thittam-primary, #3b82f6)",
                }}
              >
                <FileText className="h-5 w-5" />
              </div>
              <h3
                className="font-heading text-sm font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                From Template
              </h3>
              <p
                className="mt-1 text-xs font-body"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Start with a pre-configured budget template for your vertical.
              </p>
            </button>

            {/* Blank budget */}
            <button
              type="button"
              onClick={() => handleSelectPath("blank")}
              className={`rounded-xl p-5 text-left transition-all ${
                creationPath === "blank" ? "ring-2" : ""
              }`}
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                ...(creationPath === "blank"
                  ? { borderColor: "var(--thittam-primary, #3b82f6)", ringColor: "var(--thittam-primary, #3b82f6)" }
                  : {}),
              }}
            >
              <div
                className="mb-2 flex h-9 w-9 items-center justify-center rounded-lg"
                style={{
                  backgroundColor: "color-mix(in srgb, var(--thittam-accent, #F59E0B) 12%, transparent)",
                  color: "var(--thittam-accent, #F59E0B)",
                }}
              >
                <Plus className="h-5 w-5" />
              </div>
              <h3
                className="font-heading text-sm font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Blank Budget
              </h3>
              <p
                className="mt-1 text-xs font-body"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Start from scratch with just a label and currency.
              </p>
            </button>
          </div>

          {/* Template selector — shown when "From Template" is chosen */}
          {creationPath === "template" && (
            <div
              className="rounded-xl p-5"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              <h3
                className="mb-3 font-heading text-sm font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Select a Template
              </h3>
              <div className="flex flex-col gap-3">
                {mockTemplates.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => handleSelectTemplate(t)}
                    className={`rounded-lg p-4 text-left transition-all ${
                      selectedTemplate?.id === t.id ? "ring-2" : ""
                    }`}
                    style={{
                      backgroundColor: "var(--thittam-muted, #f1f5f9)",
                      border:
                        selectedTemplate?.id === t.id
                          ? "1px solid var(--thittam-primary, #3b82f6)"
                          : "1px solid transparent",
                    }}
                  >
                    <span
                      className="font-heading text-sm font-medium"
                      style={{ color: "var(--thittam-foreground, #0f172a)" }}
                    >
                      {t.name}
                    </span>
                    <p
                      className="mt-0.5 text-xs font-body"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      {t.description}
                    </p>
                    <span
                      className="mt-1 inline-block text-xs font-mono"
                      style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                    >
                      {t.lineItems.length} line items
                    </span>
                  </button>
                ))}
              </div>

              {/* Template preview */}
              {selectedTemplate && (
                <div className="mt-4">
                  <h4
                    className="mb-2 font-heading text-xs font-semibold uppercase tracking-wider"
                    style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                  >
                    Preview: {selectedTemplate.name}
                  </h4>
                  <div
                    className="overflow-x-auto rounded-lg border"
                    style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
                  >
                    <table className="w-full text-xs">
                      <thead>
                        <tr
                          style={{
                            backgroundColor: "var(--thittam-muted, #f1f5f9)",
                            borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                          }}
                        >
                          <th
                            className="px-3 py-2 text-left font-heading font-semibold uppercase tracking-wider"
                            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                          >
                            Category
                          </th>
                          <th
                            className="px-3 py-2 text-left font-heading font-semibold uppercase tracking-wider"
                            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                          >
                            Description
                          </th>
                          <th
                            className="px-3 py-2 text-left font-heading font-semibold uppercase tracking-wider"
                            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                          >
                            Code
                          </th>
                          <th
                            className="px-3 py-2 text-right font-heading font-semibold uppercase tracking-wider"
                            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                          >
                            Amount
                          </th>
                        </tr>
                      </thead>
                      <tbody>
                        {selectedTemplate.lineItems.map((li, idx) => (
                          <tr
                            key={idx}
                            style={{
                              borderBottom: "1px solid var(--thittam-border, #e2e8f0)",
                            }}
                          >
                            <td
                              className="px-3 py-2 font-heading"
                              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                            >
                              {li.category}
                            </td>
                            <td
                              className="px-3 py-2 font-body"
                              style={{ color: "var(--thittam-foreground, #0f172a)" }}
                            >
                              {li.description}
                            </td>
                            <td
                              className="px-3 py-2 font-mono tabular-nums"
                              style={{ color: "var(--thittam-foreground, #0f172a)" }}
                            >
                              {li.accountCode}
                            </td>
                            <td
                              className="px-3 py-2 text-right font-mono tabular-nums"
                              style={{ color: "var(--thittam-foreground, #0f172a)" }}
                            >
                              {formatINR(li.defaultAmount)}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Back button */}
          <div className="flex justify-start">
            <button
              type="button"
              onClick={() => {
                setStep(1);
                setCreationPath(null);
                setSelectedTemplate(null);
              }}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
              style={{
                color: "var(--thittam-muted-foreground, #64748b)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              Back
            </button>
          </div>
        </div>
      )}

      {/* ── Step 3: Budget details ─────────────────────────────────────── */}
      {step === 3 && (
        <form
          onSubmit={handleSubmit}
          className="rounded-xl p-6"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          {selectedTemplate && (
            <div
              className="mb-5 rounded-lg px-3 py-2"
              style={{
                backgroundColor: "color-mix(in srgb, var(--thittam-primary, #3b82f6) 8%, transparent)",
              }}
            >
              <span
                className="text-xs font-heading font-medium"
                style={{ color: "var(--thittam-primary, #3b82f6)" }}
              >
                Template: {selectedTemplate.name} ({selectedTemplate.lineItems.length} line items)
              </span>
            </div>
          )}

          <div className="flex flex-col gap-5">
            {/* Label */}
            <div>
              <label
                htmlFor="label"
                className="mb-1.5 block text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Budget Label <span className="text-red-500">*</span>
              </label>
              <input
                id="label"
                type="text"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                placeholder='e.g. "V2 — Post VFX Overrun"'
                className="w-full rounded-lg px-3 py-2 text-sm font-body"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: `1px solid var(--thittam-border, #e2e8f0)`,
                  color: "var(--thittam-foreground, #0f172a)",
                }}
              />
            </div>

            {/* Currency — only for blank budgets */}
            {!selectedTemplate && (
              <div>
                <label
                  htmlFor="currency"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Currency
                </label>
                <select
                  id="currency"
                  value={currency}
                  onChange={(e) => setCurrency(e.target.value)}
                  className="w-full rounded-lg px-3 py-2 text-sm font-mono"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                >
                  <option value="INR">INR (Indian Rupee)</option>
                  <option value="USD">USD (US Dollar)</option>
                  <option value="EUR">EUR (Euro)</option>
                  <option value="GBP">GBP (British Pound)</option>
                </select>
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="mt-8 flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={() => {
                setStep(2);
                setLabel("");
              }}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
              style={{
                color: "var(--thittam-muted-foreground, #64748b)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              Back
            </button>
            <button
              type="submit"
              disabled={isSubmitting || !label.trim()}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              {isSubmitting ? "Creating..." : "Create Budget"}
            </button>
          </div>
        </form>
      )}

      {/* Success toast */}
      {showToast && (
        <div
          className="fixed bottom-6 right-6 z-50 flex items-center gap-2 rounded-lg px-4 py-3 text-sm font-heading font-medium text-white shadow-lg"
          style={{ backgroundColor: "var(--thittam-status-on-track, #16A34A)" }}
        >
          <Check className="h-4 w-4" />
          Budget created successfully
        </div>
      )}
    </div>
  );
}
