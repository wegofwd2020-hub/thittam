"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useTheme } from "@/lib/themes/provider";
import { DepreciationBadge } from "@/components/inventory/depreciation-badge";

// ---------------------------------------------------------------------------
// Mock categories (from vertical config)
// ---------------------------------------------------------------------------

interface CategoryOption {
  id: string;
  label: string;
  depreciation_years: number | null;
}

const MOCK_CATEGORIES: CategoryOption[] = [
  { id: "cat-camera", label: "Camera Equipment", depreciation_years: 5 },
  { id: "cat-lighting", label: "Lighting Equipment", depreciation_years: 7 },
  { id: "cat-sound", label: "Sound Equipment", depreciation_years: 5 },
  { id: "cat-props", label: "Props", depreciation_years: 3 },
  { id: "cat-costumes", label: "Costumes & Wardrobe", depreciation_years: null },
  { id: "cat-vehicles", label: "Vehicles", depreciation_years: 8 },
  { id: "cat-sets", label: "Set Materials", depreciation_years: null },
];

// ---------------------------------------------------------------------------
// Form state
// ---------------------------------------------------------------------------

interface FormState {
  name: string;
  category_id: string;
  description: string;
  asset_code: string;
  serial_number: string;
  ownership_type: string;
  purchase_cost: string;
  purchase_date: string;
}

const INITIAL_FORM: FormState = {
  name: "",
  category_id: "",
  description: "",
  asset_code: "",
  serial_number: "",
  ownership_type: "owned",
  purchase_cost: "",
  purchase_date: "",
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function NewAssetPage() {
  useTheme();
  const router = useRouter();
  const [form, setForm] = useState<FormState>(INITIAL_FORM);
  const [errors, setErrors] = useState<Partial<Record<keyof FormState, string>>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  const selectedCategory = MOCK_CATEGORIES.find((c) => c.id === form.category_id);

  function validate(): boolean {
    const next: Partial<Record<keyof FormState, string>> = {};
    if (!form.name.trim()) {
      next.name = "Asset name is required";
    }
    if (!form.category_id) {
      next.category_id = "Category is required";
    }
    if (form.ownership_type === "owned" && form.purchase_cost) {
      const parsed = parseFloat(form.purchase_cost);
      if (isNaN(parsed) || parsed < 0) {
        next.purchase_cost = "Enter a valid amount";
      }
    }
    setErrors(next);
    return Object.keys(next).length === 0;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;

    setIsSubmitting(true);
    // Simulate API call — replace with real endpoint when available
    await new Promise((resolve) => setTimeout(resolve, 600));
    setIsSubmitting(false);

    router.push("/inventory");
  }

  function handleChange(field: keyof FormState, value: string) {
    setForm((prev) => ({ ...prev, [field]: value }));
    if (errors[field]) {
      setErrors((prev) => {
        const next = { ...prev };
        delete next[field];
        return next;
      });
    }
  }

  return (
    <div className="mx-auto max-w-2xl">
      {/* Back link */}
      <Link
        href="/inventory"
        className="mb-6 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Inventory
      </Link>

      {/* Page heading */}
      <h1
        className="mb-1 font-heading text-2xl font-semibold tracking-tight"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        Add Asset
      </h1>
      <p
        className="mb-8 font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Register a new equipment or asset in the inventory.
      </p>

      {/* Form */}
      <form
        onSubmit={handleSubmit}
        className="rounded-xl p-6"
        style={{
          backgroundColor: "var(--thittam-background, #fff)",
          border: "1px solid var(--thittam-border, #e2e8f0)",
        }}
      >
        <div className="flex flex-col gap-5">
          {/* Name */}
          <div>
            <label
              htmlFor="asset-name"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Name <span className="text-red-500">*</span>
            </label>
            <input
              id="asset-name"
              type="text"
              value={form.name}
              onChange={(e) => handleChange("name", e.target.value)}
              placeholder="e.g. ARRI Alexa Mini LF #3"
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: `1px solid ${errors.name ? "#DC2626" : "var(--thittam-border, #e2e8f0)"}`,
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
            {errors.name && (
              <p className="mt-1 text-xs text-red-600">{errors.name}</p>
            )}
          </div>

          {/* Category */}
          <div>
            <label
              htmlFor="asset-category"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Category <span className="text-red-500">*</span>
            </label>
            <select
              id="asset-category"
              value={form.category_id}
              onChange={(e) => handleChange("category_id", e.target.value)}
              className="w-full rounded-lg px-3 py-2 text-sm font-heading"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: `1px solid ${errors.category_id ? "#DC2626" : "var(--thittam-border, #e2e8f0)"}`,
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              <option value="">Select category...</option>
              {MOCK_CATEGORIES.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.label}
                </option>
              ))}
            </select>
            {errors.category_id && (
              <p className="mt-1 text-xs text-red-600">{errors.category_id}</p>
            )}
            {selectedCategory && (
              <div className="mt-2">
                <DepreciationBadge depreciationYears={selectedCategory.depreciation_years} />
              </div>
            )}
          </div>

          {/* Description */}
          <div>
            <label
              htmlFor="asset-description"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Description
            </label>
            <textarea
              id="asset-description"
              rows={3}
              value={form.description}
              onChange={(e) => handleChange("description", e.target.value)}
              placeholder="Additional details about this asset..."
              className="w-full resize-y rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Asset code + Serial number — side by side */}
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label
                htmlFor="asset-code"
                className="mb-1.5 block text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Asset Code
              </label>
              <input
                id="asset-code"
                type="text"
                value={form.asset_code}
                onChange={(e) => handleChange("asset_code", e.target.value)}
                placeholder="Auto-generated if blank"
                className="w-full rounded-lg px-3 py-2 text-sm font-mono"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                  color: "var(--thittam-foreground, #0f172a)",
                }}
              />
            </div>
            <div>
              <label
                htmlFor="serial-number"
                className="mb-1.5 block text-sm font-heading font-medium"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Serial Number
              </label>
              <input
                id="serial-number"
                type="text"
                value={form.serial_number}
                onChange={(e) => handleChange("serial_number", e.target.value)}
                placeholder="e.g. ARRI-K1234567"
                className="w-full rounded-lg px-3 py-2 text-sm font-mono"
                style={{
                  backgroundColor: "var(--thittam-background, #fff)",
                  border: "1px solid var(--thittam-border, #e2e8f0)",
                  color: "var(--thittam-foreground, #0f172a)",
                }}
              />
            </div>
          </div>

          {/* Ownership type */}
          <div>
            <span
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Ownership Type
            </span>
            <div className="flex items-center gap-4">
              {(["owned", "rented", "borrowed"] as const).map((opt) => (
                <label key={opt} className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="ownership_type"
                    value={opt}
                    checked={form.ownership_type === opt}
                    onChange={() => handleChange("ownership_type", opt)}
                    className="accent-[var(--thittam-primary,#3b82f6)]"
                  />
                  <span
                    className="text-sm font-body capitalize"
                    style={{ color: "var(--thittam-foreground, #0f172a)" }}
                  >
                    {opt}
                  </span>
                </label>
              ))}
            </div>
          </div>

          {/* Purchase cost + date — only for owned assets */}
          {form.ownership_type === "owned" && (
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label
                  htmlFor="purchase-cost"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Purchase Cost
                </label>
                <input
                  id="purchase-cost"
                  type="text"
                  inputMode="decimal"
                  value={form.purchase_cost}
                  onChange={(e) => handleChange("purchase_cost", e.target.value)}
                  placeholder="0.00"
                  className="w-full rounded-lg px-3 py-2 text-sm font-mono tabular-nums"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: `1px solid ${errors.purchase_cost ? "#DC2626" : "var(--thittam-border, #e2e8f0)"}`,
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
                {errors.purchase_cost && (
                  <p className="mt-1 text-xs text-red-600">{errors.purchase_cost}</p>
                )}
              </div>
              <div>
                <label
                  htmlFor="purchase-date"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Purchase Date
                </label>
                <input
                  id="purchase-date"
                  type="date"
                  value={form.purchase_date}
                  onChange={(e) => handleChange("purchase_date", e.target.value)}
                  className="w-full rounded-lg px-3 py-2 text-sm font-mono"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="mt-8 flex items-center justify-end gap-3">
          <Link
            href="/inventory"
            className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
            style={{
              color: "var(--thittam-muted-foreground, #64748b)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            Cancel
          </Link>
          <button
            type="submit"
            disabled={isSubmitting}
            className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            {isSubmitting ? "Adding..." : "Add Asset"}
          </button>
        </div>
      </form>
    </div>
  );
}
