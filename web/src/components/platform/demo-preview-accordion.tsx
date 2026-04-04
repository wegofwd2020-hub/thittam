"use client";

import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Building,
  Users,
  BookOpen,
  Layers,
  FileText,
  Receipt,
  ShieldCheck,
  Package,
  FolderOpen,
} from "lucide-react";
import type { DemoPreview } from "@/lib/api/platform";

// ---------------------------------------------------------------------------
// DemoPreviewAccordion — collapsible sections for the preview step.
// ---------------------------------------------------------------------------

interface DemoPreviewAccordionProps {
  preview: DemoPreview;
  onCompanyNameChange: (name: string) => void;
}

interface SectionProps {
  title: string;
  icon: React.ReactNode;
  count?: number;
  defaultOpen?: boolean;
  children: React.ReactNode;
}

function Section({ title, icon, count, defaultOpen = true, children }: SectionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

  return (
    <div
      className="overflow-hidden rounded-lg border"
      style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
    >
      <button
        type="button"
        onClick={() => setIsOpen((o) => !o)}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-gray-50"
        style={{ backgroundColor: isOpen ? "var(--thittam-muted, #f1f5f9)" : "white" }}
      >
        {isOpen ? (
          <ChevronDown className="h-4 w-4 shrink-0" style={{ color: "#64748b" }} />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0" style={{ color: "#64748b" }} />
        )}
        <span className="flex items-center gap-2 text-sm font-semibold font-heading" style={{ color: "#0f172a" }}>
          {icon}
          {title}
        </span>
        {count !== undefined && (
          <span className="ml-auto inline-flex items-center rounded-full bg-gray-200 px-2 py-0.5 text-xs font-mono tabular-nums" style={{ color: "#475569" }}>
            {count}
          </span>
        )}
      </button>
      {isOpen && (
        <div className="border-t px-4 py-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
          {children}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Simple table helper for the accordion sections.
// ---------------------------------------------------------------------------

function MiniTable({
  headers,
  rows,
}: {
  headers: string[];
  rows: React.ReactNode[][];
}) {
  return (
    <div className="overflow-x-auto rounded-lg border" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
      <table className="w-full text-sm">
        <thead>
          <tr style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}>
            {headers.map((h) => (
              <th
                key={h}
                className="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wider font-heading"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, idx) => (
            <tr
              key={idx}
              className={idx % 2 === 1 ? "bg-gray-50/50" : ""}
              style={{ borderTop: "1px solid var(--thittam-border, #e2e8f0)" }}
            >
              {row.map((cell, ci) => (
                <td key={ci} className="px-3 py-2 font-body" style={{ color: "#0f172a" }}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function DemoPreviewAccordion({
  preview,
  onCompanyNameChange,
}: DemoPreviewAccordionProps) {
  return (
    <div className="space-y-3">
      {/* Tenant Info */}
      <Section
        title="Tenant Info"
        icon={<Building className="h-4 w-4" style={{ color: "#6366f1" }} />}
      >
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
              Company Name
            </label>
            <input
              type="text"
              value={preview.tenant.name}
              onChange={(e) => onCompanyNameChange(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-body focus:outline-none focus:ring-2"
              style={{
                borderColor: "var(--thittam-border, #e2e8f0)",
                color: "#0f172a",
              }}
            />
          </div>
          <div>
            <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
              Slug
            </label>
            <p className="mt-1 text-sm font-mono" style={{ color: "#0f172a" }}>
              {preview.tenant.slug}
            </p>
          </div>
          <div>
            <label className="block text-xs font-semibold font-heading" style={{ color: "#64748b" }}>
              Plan
            </label>
            <p className="mt-1 text-sm font-heading" style={{ color: "#0f172a" }}>
              {preview.tenant.plan}
            </p>
          </div>
        </div>
      </Section>

      {/* Users */}
      <Section
        title="Users"
        icon={<Users className="h-4 w-4" style={{ color: "#2563eb" }} />}
        count={preview.users.length}
      >
        <MiniTable
          headers={["Name", "Email", "Role", "Department", "Temp Password"]}
          rows={preview.users.map((u) => [
            <span key="n" className="font-heading font-medium">{u.name}</span>,
            <span key="e" className="font-mono text-xs">{u.email}</span>,
            <span key="r" className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-heading text-blue-700">
              {u.role}
            </span>,
            <span key="d" className="font-body">{u.department}</span>,
            <span key="p" className="font-mono text-xs">{u.temp_password}</span>,
          ])}
        />
      </Section>

      {/* Chart of Accounts */}
      <Section
        title="Chart of Accounts"
        icon={<BookOpen className="h-4 w-4" style={{ color: "#16a34a" }} />}
        count={preview.chart_of_accounts.length}
      >
        <MiniTable
          headers={["Code", "Name", "Type"]}
          rows={preview.chart_of_accounts.map((a) => [
            <span key="c" className="font-mono text-xs">{a.code}</span>,
            <span key="n" className="font-body">{a.name}</span>,
            <span key="t" className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-heading text-gray-700">
              {a.account_type}
            </span>,
          ])}
        />
      </Section>

      {/* Budget Categories */}
      <Section
        title="Budget Categories"
        icon={<Layers className="h-4 w-4" style={{ color: "#d97706" }} />}
        count={preview.budget_categories.length}
      >
        <div className="space-y-2">
          {preview.budget_categories.map((cat) => (
            <div key={cat.id} className="rounded-md border px-3 py-2" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <p className="text-sm font-semibold font-heading" style={{ color: "#0f172a" }}>{cat.label}</p>
              <p className="text-xs font-body" style={{ color: "#64748b" }}>{cat.description}</p>
            </div>
          ))}
        </div>
      </Section>

      {/* Budget Templates */}
      <Section
        title="Budget Templates"
        icon={<FileText className="h-4 w-4" style={{ color: "#7c3aed" }} />}
        count={preview.budget_templates.length}
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {preview.budget_templates.map((t) => (
            <div key={t.name} className="rounded-lg border p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <p className="text-sm font-semibold font-heading" style={{ color: "#0f172a" }}>{t.name}</p>
              <p className="mt-1 text-xs font-body" style={{ color: "#64748b" }}>{t.description}</p>
              <p className="mt-2 text-xs font-mono tabular-nums" style={{ color: "#94a3b8" }}>
                {t.line_items.length} line items
              </p>
            </div>
          ))}
        </div>
      </Section>

      {/* Expense Categories */}
      <Section
        title="Expense Categories"
        icon={<Receipt className="h-4 w-4" style={{ color: "#dc2626" }} />}
        count={preview.expense_categories.length}
      >
        <MiniTable
          headers={["Category", "Tax Treatment", "Requires PO"]}
          rows={preview.expense_categories.map((ec) => [
            <span key="l" className="font-heading font-medium">{ec.label}</span>,
            <span
              key="t"
              className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-heading ${
                ec.tax_treatment === "deductible"
                  ? "bg-green-100 text-green-700"
                  : ec.tax_treatment === "non-deductible"
                    ? "bg-red-100 text-red-700"
                    : "bg-gray-100 text-gray-700"
              }`}
            >
              {ec.tax_treatment}
            </span>,
            <span
              key="p"
              className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-heading ${
                ec.requires_po ? "bg-amber-100 text-amber-700" : "bg-gray-100 text-gray-500"
              }`}
            >
              {ec.requires_po ? "Yes" : "No"}
            </span>,
          ])}
        />
      </Section>

      {/* Approval Workflow */}
      <Section
        title="Approval Workflow"
        icon={<ShieldCheck className="h-4 w-4" style={{ color: "#0891b2" }} />}
      >
        <MiniTable
          headers={["Role", "Max Amount"]}
          rows={preview.approval_workflow.limits.map((l) => [
            <span key="r" className="font-heading font-medium">{l.role}</span>,
            <span key="a" className="font-mono tabular-nums">${l.max_amount}</span>,
          ])}
        />
        <p className="mt-3 text-xs font-body" style={{ color: "#64748b" }}>
          Dual approval required above{" "}
          <span className="font-mono font-semibold tabular-nums">
            ${preview.approval_workflow.dual_approval_above}
          </span>
        </p>
      </Section>

      {/* Inventory Categories */}
      <Section
        title="Inventory Categories"
        icon={<Package className="h-4 w-4" style={{ color: "#059669" }} />}
        count={preview.inventory_categories.length}
      >
        <div className="flex flex-wrap gap-2">
          {preview.inventory_categories.map((ic) => (
            <div
              key={ic.id}
              className="inline-flex items-center gap-2 rounded-full border px-3 py-1.5"
              style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
            >
              <span className="text-sm font-heading" style={{ color: "#0f172a" }}>{ic.label}</span>
              {ic.is_trackable && (
                <span className="inline-flex items-center rounded-full bg-emerald-100 px-1.5 py-0.5 text-[10px] font-heading text-emerald-700">
                  Trackable
                </span>
              )}
            </div>
          ))}
        </div>
      </Section>

      {/* Sample Projects */}
      <Section
        title={`Sample ${preview.entity_labels.projectPlural}`}
        icon={<FolderOpen className="h-4 w-4" style={{ color: "#8b5cf6" }} />}
        count={preview.projects.length}
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {preview.projects.map((p) => (
            <div key={p.title} className="rounded-lg border p-4" style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}>
              <p className="text-sm font-semibold font-heading" style={{ color: "#0f172a" }}>{p.title}</p>
              <p className="mt-1 text-xs font-body" style={{ color: "#64748b" }}>{p.description}</p>
              <div className="mt-3 flex items-center gap-2">
                <span className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-heading text-blue-700">
                  {p.phase}
                </span>
                <span className="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-heading text-green-700">
                  {p.status}
                </span>
              </div>
              <div className="mt-2 flex items-center justify-between text-xs" style={{ color: "#94a3b8" }}>
                <span className="font-mono tabular-nums">${p.budget_total}</span>
                <span className="font-mono tabular-nums">{p.expense_count} expenses</span>
              </div>
            </div>
          ))}
        </div>
      </Section>
    </div>
  );
}
