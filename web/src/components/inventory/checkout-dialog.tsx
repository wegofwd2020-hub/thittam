"use client";

import { useState } from "react";
import { X } from "lucide-react";

// ---------------------------------------------------------------------------
// CheckoutDialog — dialog for checking out an asset to a production.
// ---------------------------------------------------------------------------

interface CheckoutDialogProps {
  assetName: string;
  onConfirm: (data: {
    production_id: string;
    checked_out_to: string;
    expected_return: string;
    condition_out: string;
  }) => void;
  onCancel: () => void;
}

const MOCK_PRODUCTIONS = [
  { id: "prod-001", title: "The Last Horizon" },
  { id: "prod-002", title: "Monsoon Chronicles" },
  { id: "prod-003", title: "Neon Nights" },
];

export function CheckoutDialog({
  assetName,
  onConfirm,
  onCancel,
}: CheckoutDialogProps) {
  const [productionId, setProductionId] = useState("");
  const [person, setPerson] = useState("");
  const [expectedReturn, setExpectedReturn] = useState("");
  const [condition, setCondition] = useState("good");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!productionId || !person) return;
    onConfirm({
      production_id: productionId,
      checked_out_to: person,
      expected_return: expectedReturn,
      condition_out: condition,
    });
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30" onClick={onCancel} />
      <div
        className="relative w-full max-w-md rounded-xl p-6 shadow-xl"
        style={{ backgroundColor: "var(--thittam-background, #fff)" }}
      >
        {/* Header */}
        <div className="mb-4 flex items-center justify-between">
          <h2
            className="font-heading text-lg font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Check Out Asset
          </h2>
          <button
            onClick={onCancel}
            className="rounded-md p-1 transition-colors hover:bg-gray-100"
          >
            <X
              className="h-5 w-5"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            />
          </button>
        </div>

        <p
          className="mb-4 font-body text-sm"
          style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
        >
          Checking out <strong className="font-heading">{assetName}</strong>
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {/* Production */}
          <div>
            <label
              htmlFor="checkout-production"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Production <span className="text-red-500">*</span>
            </label>
            <select
              id="checkout-production"
              value={productionId}
              onChange={(e) => setProductionId(e.target.value)}
              className="w-full rounded-lg px-3 py-2 text-sm font-heading"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            >
              <option value="">Select production...</option>
              {MOCK_PRODUCTIONS.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.title}
                </option>
              ))}
            </select>
          </div>

          {/* Person */}
          <div>
            <label
              htmlFor="checkout-person"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Checked Out To <span className="text-red-500">*</span>
            </label>
            <input
              id="checkout-person"
              type="text"
              value={person}
              onChange={(e) => setPerson(e.target.value)}
              placeholder="Person name"
              className="w-full rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Expected return */}
          <div>
            <label
              htmlFor="checkout-return"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Expected Return Date
            </label>
            <input
              id="checkout-return"
              type="date"
              value={expectedReturn}
              onChange={(e) => setExpectedReturn(e.target.value)}
              className="w-full rounded-lg px-3 py-2 text-sm font-mono"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Condition */}
          <div>
            <span
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Condition at Checkout
            </span>
            <div className="flex items-center gap-4">
              {(["good", "fair"] as const).map((opt) => (
                <label key={opt} className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="condition_out"
                    value={opt}
                    checked={condition === opt}
                    onChange={() => setCondition(opt)}
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

          {/* Actions */}
          <div className="mt-2 flex justify-end gap-3">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium transition-colors"
              style={{
                color: "var(--thittam-muted-foreground, #64748b)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!productionId || !person}
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              Confirm Check Out
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
