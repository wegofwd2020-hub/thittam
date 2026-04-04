"use client";

import { useState } from "react";
import { Check } from "lucide-react";

export interface Phase {
  id: string;
  type: string;
  label: string;
  status: "completed" | "current" | "upcoming";
  billable: boolean;
  startDate?: string;
  endDate?: string;
}

interface AllowedTransition {
  from: string;
  to: string;
}

interface PhaseTimelineProps {
  phases: Phase[];
  allowedTransitions: AllowedTransition[];
  onTransition?: (fromPhaseId: string, toPhaseId: string) => void;
}

export function PhaseTimeline({
  phases,
  allowedTransitions,
  onTransition,
}: PhaseTimelineProps) {
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);

  const currentPhase = phases.find((p) => p.status === "current");

  function canTransitionTo(targetPhaseId: string): boolean {
    if (!currentPhase) return false;
    return allowedTransitions.some(
      (t) => t.from === currentPhase.id && t.to === targetPhaseId
    );
  }

  function handlePhaseClick(phase: Phase) {
    if (phase.status !== "upcoming" || !canTransitionTo(phase.id)) return;
    setConfirmTarget(phase.id);
  }

  function handleConfirm() {
    if (!currentPhase || !confirmTarget) return;
    onTransition?.(currentPhase.id, confirmTarget);
    setConfirmTarget(null);
  }

  return (
    <div className="relative">
      {/* Horizontal timeline */}
      <div className="flex items-start gap-0 overflow-x-auto pb-4">
        {phases.map((phase, idx) => {
          const isLast = idx === phases.length - 1;
          const isClickable =
            phase.status === "upcoming" && canTransitionTo(phase.id);

          return (
            <div key={phase.id} className="flex items-start">
              {/* Phase node */}
              <button
                onClick={() => handlePhaseClick(phase)}
                disabled={!isClickable}
                className={`flex flex-col items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                  isClickable
                    ? "cursor-pointer hover:bg-gray-50"
                    : "cursor-default"
                }`}
                style={{ minWidth: "120px" }}
              >
                {/* Circle indicator */}
                <div
                  className={`flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold font-heading ${
                    phase.status === "completed"
                      ? "bg-green-100 text-green-700"
                      : phase.status === "current"
                        ? "text-white"
                        : "bg-gray-100 text-gray-400"
                  }`}
                  style={
                    phase.status === "current"
                      ? {
                          backgroundColor:
                            "var(--thittam-primary, #3b82f6)",
                        }
                      : undefined
                  }
                >
                  {phase.status === "completed" ? (
                    <Check className="h-4 w-4" />
                  ) : (
                    idx + 1
                  )}
                </div>

                {/* Label */}
                <span
                  className={`text-xs font-heading font-medium text-center ${
                    phase.status === "current"
                      ? "font-semibold"
                      : phase.status === "upcoming"
                        ? "opacity-50"
                        : ""
                  }`}
                  style={{
                    color:
                      phase.status === "current"
                        ? "var(--thittam-primary, #3b82f6)"
                        : "var(--thittam-foreground, #0f172a)",
                  }}
                >
                  {phase.label}
                </span>

                {/* Billable badge */}
                {phase.billable && (
                  <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-[10px] font-heading font-medium text-emerald-700">
                    Billable
                  </span>
                )}

                {/* Dates */}
                {phase.startDate && (
                  <span
                    className="font-mono text-[10px]"
                    style={{
                      color: "var(--thittam-muted-foreground, #64748b)",
                    }}
                  >
                    {phase.startDate}
                    {phase.endDate ? ` - ${phase.endDate}` : ""}
                  </span>
                )}
              </button>

              {/* Arrow connector */}
              {!isLast && (
                <div className="flex items-center self-center pt-1">
                  <div
                    className="h-0.5 w-8"
                    style={{
                      backgroundColor: "var(--thittam-border, #e2e8f0)",
                    }}
                  />
                  <div
                    className="h-0 w-0 border-y-[4px] border-l-[6px] border-y-transparent"
                    style={{
                      borderLeftColor: "var(--thittam-border, #e2e8f0)",
                    }}
                  />
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Confirm dialog */}
      {confirmTarget && (
        <div
          className="mt-4 flex items-center gap-3 rounded-lg p-4"
          style={{
            backgroundColor: "var(--thittam-muted, #f1f5f9)",
            border: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          <p
            className="flex-1 text-sm font-body"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Transition to{" "}
            <strong className="font-heading">
              {phases.find((p) => p.id === confirmTarget)?.label}
            </strong>
            ?
          </p>
          <button
            onClick={handleConfirm}
            className="rounded-md px-3 py-1.5 text-xs font-heading font-medium text-white"
            style={{
              backgroundColor: "var(--thittam-primary, #3b82f6)",
            }}
          >
            Confirm
          </button>
          <button
            onClick={() => setConfirmTarget(null)}
            className="rounded-md px-3 py-1.5 text-xs font-heading font-medium"
            style={{
              color: "var(--thittam-muted-foreground, #64748b)",
              border: "1px solid var(--thittam-border, #e2e8f0)",
            }}
          >
            Cancel
          </button>
        </div>
      )}
    </div>
  );
}
