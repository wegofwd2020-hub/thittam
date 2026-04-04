"use client";

import { Check, X, Loader2 } from "lucide-react";

// ---------------------------------------------------------------------------
// ProvisionProgress — step-by-step progress indicator.
// ---------------------------------------------------------------------------

interface ProvisionStep {
  label: string;
  status: "pending" | "active" | "completed" | "failed";
}

interface ProvisionProgressProps {
  currentStep: number;
  steps: ProvisionStep[];
}

export function ProvisionProgress({ steps }: ProvisionProgressProps) {
  return (
    <div className="space-y-0">
      {steps.map((step, idx) => {
        const isLast = idx === steps.length - 1;

        return (
          <div key={step.label} className="flex gap-3">
            {/* Indicator column */}
            <div className="flex flex-col items-center">
              {/* Circle */}
              <div
                className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-all duration-300 ${
                  step.status === "completed"
                    ? "bg-green-500 text-white"
                    : step.status === "active"
                      ? "bg-blue-500 text-white"
                      : step.status === "failed"
                        ? "bg-red-500 text-white"
                        : "bg-gray-200 text-gray-400"
                }`}
              >
                {step.status === "completed" && <Check className="h-4 w-4" />}
                {step.status === "active" && <Loader2 className="h-4 w-4 animate-spin" />}
                {step.status === "failed" && <X className="h-4 w-4" />}
                {step.status === "pending" && (
                  <span className="text-xs font-mono tabular-nums">{idx + 1}</span>
                )}
              </div>
              {/* Connector line */}
              {!isLast && (
                <div
                  className={`w-0.5 flex-1 min-h-[24px] transition-colors duration-300 ${
                    step.status === "completed"
                      ? "bg-green-300"
                      : step.status === "failed"
                        ? "bg-red-300"
                        : "bg-gray-200"
                  }`}
                />
              )}
            </div>

            {/* Label */}
            <div className="flex items-start pb-6">
              <p
                className={`text-sm font-heading pt-1.5 transition-colors duration-300 ${
                  step.status === "completed"
                    ? "text-green-700 font-medium"
                    : step.status === "active"
                      ? "text-blue-700 font-semibold"
                      : step.status === "failed"
                        ? "text-red-700 font-medium"
                        : "text-gray-400"
                }`}
              >
                {step.label}
                {step.status === "active" && (
                  <span className="ml-2 text-xs text-blue-400 font-body">in progress...</span>
                )}
              </p>
            </div>
          </div>
        );
      })}
    </div>
  );
}
