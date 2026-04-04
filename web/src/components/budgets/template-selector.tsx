"use client";

import type { BudgetTemplate } from "@/lib/api/budgets";

// ---------------------------------------------------------------------------
// TemplateSelector — grid of template cards for budget creation.
// ---------------------------------------------------------------------------

interface TemplateSelectorProps {
  templates: BudgetTemplate[];
  onSelect: (template: BudgetTemplate) => void;
  selectedName?: string;
}

function sumDefaultAmounts(template: BudgetTemplate): number {
  return template.line_items.reduce((sum, item) => {
    const val = parseFloat(item.default_amount);
    return sum + (isNaN(val) ? 0 : val);
  }, 0);
}

export function TemplateSelector({
  templates,
  onSelect,
  selectedName,
}: TemplateSelectorProps) {
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {templates.map((template) => {
          const isSelected = selectedName === template.name;
          const total = sumDefaultAmounts(template);

          return (
            <button
              key={template.name}
              type="button"
              onClick={() => onSelect(template)}
              className={`rounded-lg border p-4 text-left transition-colors hover:shadow-sm ${
                isSelected ? "ring-2 ring-offset-1" : ""
              }`}
              style={{
                borderColor: isSelected
                  ? "var(--thittam-primary, #2563eb)"
                  : "var(--thittam-border, #e2e8f0)",
                backgroundColor: "var(--thittam-card, #ffffff)",
                // ring color via inline for CSS var support
                ...(isSelected
                  ? { ["--tw-ring-color" as string]: "var(--thittam-primary, #2563eb)" }
                  : {}),
              }}
            >
              <h4
                className="text-sm font-semibold font-heading"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {template.name}
              </h4>
              <p
                className="mt-1 text-xs font-body line-clamp-2"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                {template.description}
              </p>
              <div
                className="mt-3 flex items-center justify-between text-xs"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                <span className="font-heading">
                  {template.line_items.length} line item
                  {template.line_items.length !== 1 ? "s" : ""}
                </span>
                <span className="font-mono tabular-nums">
                  ${total.toLocaleString("en-US", {
                    minimumFractionDigits: 2,
                    maximumFractionDigits: 2,
                  })}
                </span>
              </div>
            </button>
          );
        })}

        {/* Blank budget option */}
        <button
          type="button"
          onClick={() =>
            onSelect({
              name: "",
              description: "Start with a blank budget",
              line_items: [],
            })
          }
          className={`flex flex-col items-center justify-center rounded-lg border border-dashed p-4 transition-colors hover:shadow-sm ${
            selectedName === "" ? "ring-2 ring-offset-1" : ""
          }`}
          style={{
            borderColor: selectedName === ""
              ? "var(--thittam-primary, #2563eb)"
              : "var(--thittam-border, #e2e8f0)",
            backgroundColor: "var(--thittam-card, #ffffff)",
            ...(selectedName === ""
              ? { ["--tw-ring-color" as string]: "var(--thittam-primary, #2563eb)" }
              : {}),
          }}
        >
          <span
            className="text-2xl"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            aria-hidden="true"
          >
            +
          </span>
          <span
            className="mt-1 text-sm font-heading"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            Or create blank budget
          </span>
        </button>
      </div>
    </div>
  );
}
