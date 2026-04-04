"use client";

import { useState } from "react";
import { X } from "lucide-react";

// ---------------------------------------------------------------------------
// CheckinDialog — dialog for checking in an asset with condition assessment.
// ---------------------------------------------------------------------------

interface CheckinDialogProps {
  assetName: string;
  onConfirm: (data: {
    condition_in: string;
    notes: string;
    report_damage: boolean;
    damage_severity?: string;
    damage_description?: string;
    repair_cost?: string;
  }) => void;
  onCancel: () => void;
}

export function CheckinDialog({
  assetName,
  onConfirm,
  onCancel,
}: CheckinDialogProps) {
  const [condition, setCondition] = useState("good");
  const [notes, setNotes] = useState("");
  const [reportDamage, setReportDamage] = useState(false);
  const [damageSeverity, setDamageSeverity] = useState("minor");
  const [damageDescription, setDamageDescription] = useState("");
  const [repairCost, setRepairCost] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    onConfirm({
      condition_in: condition,
      notes,
      report_damage: reportDamage,
      ...(reportDamage && {
        damage_severity: damageSeverity,
        damage_description: damageDescription,
        repair_cost: repairCost || undefined,
      }),
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
            Check In Asset
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
          Checking in <strong className="font-heading">{assetName}</strong>
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {/* Condition assessment */}
          <div>
            <span
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Condition Assessment <span className="text-red-500">*</span>
            </span>
            <div className="flex items-center gap-4">
              {(["good", "fair", "damaged"] as const).map((opt) => (
                <label key={opt} className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="condition_in"
                    value={opt}
                    checked={condition === opt}
                    onChange={() => {
                      setCondition(opt);
                      if (opt === "damaged") setReportDamage(true);
                    }}
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

          {/* Notes */}
          <div>
            <label
              htmlFor="checkin-notes"
              className="mb-1.5 block text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Notes
            </label>
            <textarea
              id="checkin-notes"
              rows={3}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Any observations about the asset condition..."
              className="w-full resize-y rounded-lg px-3 py-2 text-sm font-body"
              style={{
                backgroundColor: "var(--thittam-background, #fff)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
              }}
            />
          </div>

          {/* Report damage checkbox */}
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={reportDamage}
              onChange={(e) => setReportDamage(e.target.checked)}
              className="accent-[var(--thittam-primary,#3b82f6)]"
            />
            <span
              className="text-sm font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Report Damage
            </span>
          </label>

          {/* Damage form — expanded when checked */}
          {reportDamage && (
            <div
              className="flex flex-col gap-3 rounded-lg p-3"
              style={{
                backgroundColor: "var(--thittam-muted, #f1f5f9)",
                border: "1px solid var(--thittam-border, #e2e8f0)",
              }}
            >
              {/* Severity */}
              <div>
                <span
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Damage Severity
                </span>
                <div className="flex items-center gap-4">
                  {(["minor", "major", "total_loss"] as const).map((opt) => (
                    <label key={opt} className="flex items-center gap-2">
                      <input
                        type="radio"
                        name="damage_severity"
                        value={opt}
                        checked={damageSeverity === opt}
                        onChange={() => setDamageSeverity(opt)}
                        className="accent-[var(--thittam-primary,#3b82f6)]"
                      />
                      <span
                        className="text-sm font-body capitalize"
                        style={{ color: "var(--thittam-foreground, #0f172a)" }}
                      >
                        {opt.replace(/_/g, " ")}
                      </span>
                    </label>
                  ))}
                </div>
              </div>

              {/* Description */}
              <div>
                <label
                  htmlFor="damage-description"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Damage Description
                </label>
                <textarea
                  id="damage-description"
                  rows={2}
                  value={damageDescription}
                  onChange={(e) => setDamageDescription(e.target.value)}
                  placeholder="Describe the damage..."
                  className="w-full resize-y rounded-lg px-3 py-2 text-sm font-body"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>

              {/* Estimated repair cost */}
              <div>
                <label
                  htmlFor="repair-cost"
                  className="mb-1.5 block text-sm font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Estimated Repair Cost
                </label>
                <input
                  id="repair-cost"
                  type="text"
                  inputMode="decimal"
                  value={repairCost}
                  onChange={(e) => setRepairCost(e.target.value)}
                  placeholder="0.00"
                  className="w-full rounded-lg px-3 py-2 text-sm font-mono tabular-nums"
                  style={{
                    backgroundColor: "var(--thittam-background, #fff)",
                    border: "1px solid var(--thittam-border, #e2e8f0)",
                    color: "var(--thittam-foreground, #0f172a)",
                  }}
                />
              </div>
            </div>
          )}

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
              className="rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90"
              style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
            >
              Confirm Check In
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
