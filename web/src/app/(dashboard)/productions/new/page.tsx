"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useTheme } from "@/lib/themes/provider";

interface FormState {
  title: string;
  description: string;
  genre: string;
  startDate: string;
}

const INITIAL_FORM: FormState = {
  title: "",
  description: "",
  genre: "",
  startDate: "",
};

export default function NewProductionPage() {
  const { entityLabels } = useTheme();
  const router = useRouter();
  const [form, setForm] = useState<FormState>(INITIAL_FORM);
  const [errors, setErrors] = useState<Partial<Record<keyof FormState, string>>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  function validate(): boolean {
    const next: Partial<Record<keyof FormState, string>> = {};
    if (!form.title.trim()) {
      next.title = `${entityLabels.project} title is required`;
    }
    if (form.title.trim().length > 0 && form.title.trim().length < 3) {
      next.title = "Title must be at least 3 characters";
    }
    if (!form.startDate) {
      next.startDate = "Start date is required";
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

    // Navigate back to list after "creation"
    router.push("/productions");
  }

  function handleChange(
    field: keyof FormState,
    value: string
  ) {
    setForm((prev) => ({ ...prev, [field]: value }));
    // Clear error on change
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
        href="/productions"
        className="mb-6 inline-flex items-center gap-1.5 text-sm font-heading font-medium transition-colors hover:opacity-80"
        style={{ color: "var(--thittam-primary, #3b82f6)" }}
      >
        <ArrowLeft className="h-4 w-4" />
        Back to {entityLabels.projectPlural}
      </Link>

      {/* Page heading */}
      <h1
        className="mb-1 font-heading text-2xl font-semibold tracking-tight"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        Create {entityLabels.project}
      </h1>
      <p
        className="mb-8 font-body text-sm"
        style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
      >
        Fill in the details below to set up a new{" "}
        {entityLabels.project.toLowerCase()}.
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
          {/* Title */}
          <div>
            <label
              htmlFor="title"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Title <span className="text-red-500">*</span>
            </label>
            <input
              id="title"
              type="text"
              value={form.title}
              onChange={(e) => handleChange("title", e.target.value)}
              placeholder={`e.g. My New ${entityLabels.project}`}
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: `1px solid ${errors.title ? "#DC2626" : "var(--thittam-border, #e2e8f0)"}`,
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
            {errors.title && (
              <p className="mt-1 text-xs text-red-600">{errors.title}</p>
            )}
          </div>

          {/* Description */}
          <div>
            <label
              htmlFor="description"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Description
            </label>
            <textarea
              id="description"
              rows={4}
              value={form.description}
              onChange={(e) => handleChange("description", e.target.value)}
              placeholder={`Describe the ${entityLabels.project.toLowerCase()}...`}
              className="w-full resize-y rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Genre / Type */}
          <div>
            <label
              htmlFor="genre"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Genre / Type
            </label>
            <input
              id="genre"
              type="text"
              value={form.genre}
              onChange={(e) => handleChange("genre", e.target.value)}
              placeholder="e.g. Sci-Fi, Drama, Documentary"
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Start Date */}
          <div>
            <label
              htmlFor="startDate"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Start Date <span className="text-red-500">*</span>
            </label>
            <input
              id="startDate"
              type="date"
              value={form.startDate}
              onChange={(e) => handleChange("startDate", e.target.value)}
              className="w-full rounded-lg px-3 py-2 text-sm font-mono"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: `1px solid ${errors.startDate ? "#DC2626" : "var(--thittam-border, #e2e8f0)"}`,
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
            {errors.startDate && (
              <p className="mt-1 text-xs text-red-600">{errors.startDate}</p>
            )}
          </div>
        </div>

        {/* Actions */}
        <div className="mt-8 flex items-center justify-end gap-3">
          <Link
            href="/productions"
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
            {isSubmitting
              ? "Creating..."
              : `Create ${entityLabels.project}`}
          </button>
        </div>
      </form>
    </div>
  );
}
