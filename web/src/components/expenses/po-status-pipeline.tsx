"use client";

// ---------------------------------------------------------------------------
// POStatusPipeline — horizontal step indicator for purchase order status.
// ---------------------------------------------------------------------------

interface POStatusPipelineProps {
  status: string; // draft | approved | partially_invoiced | closed | cancelled
}

const STEPS = [
  { key: "draft", label: "Draft" },
  { key: "approved", label: "Approved" },
  { key: "partially_invoiced", label: "Partially Invoiced" },
  { key: "closed", label: "Closed" },
] as const;

const STEP_INDEX: Record<string, number> = {
  draft: 0,
  approved: 1,
  partially_invoiced: 2,
  closed: 3,
};

export function POStatusPipeline({ status }: POStatusPipelineProps) {
  const isCancelled = status === "cancelled";
  const currentIndex = STEP_INDEX[status] ?? -1;

  if (isCancelled) {
    return (
      <div className="flex items-center gap-2">
        {/* Show the normal pipeline greyed out with a cancelled branch */}
        {STEPS.map((step, i) => (
          <div key={step.key} className="flex items-center gap-2">
            {i > 0 && (
              <div
                className="h-0.5 w-6"
                style={{ backgroundColor: "rgb(229 231 235)" }} // gray-200
              />
            )}
            <div className="flex flex-col items-center gap-1">
              <div
                className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-heading"
                style={{
                  backgroundColor: "rgb(243 244 246)", // gray-100
                  color: "rgb(156 163 175)", // gray-400
                }}
              >
                {i + 1}
              </div>
              <span
                className="text-xs font-body"
                style={{ color: "rgb(156 163 175)" }} // gray-400
              >
                {step.label}
              </span>
            </div>
          </div>
        ))}

        {/* Cancelled branch */}
        <div className="flex items-center gap-2">
          <div
            className="h-0.5 w-6"
            style={{ backgroundColor: "rgb(252 165 165)" }} // red-300
          />
          <div className="flex flex-col items-center gap-1">
            <div
              className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-heading"
              style={{
                backgroundColor: "rgb(254 226 226)", // red-100
                color: "rgb(153 27 27)", // red-800
              }}
            >
              &times;
            </div>
            <span
              className="text-xs font-heading"
              style={{ color: "rgb(153 27 27)" }} // red-800
            >
              Cancelled
            </span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      {STEPS.map((step, i) => {
        const isCompleted = i < currentIndex;
        const isCurrent = i === currentIndex;

        return (
          <div key={step.key} className="flex items-center gap-2">
            {i > 0 && (
              <div
                className="h-0.5 w-6"
                style={{
                  backgroundColor: isCompleted
                    ? "rgb(34 197 94)" // green-500
                    : "rgb(229 231 235)", // gray-200
                }}
              />
            )}
            <div className="flex flex-col items-center gap-1">
              <div
                className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-heading"
                style={
                  isCompleted
                    ? {
                        backgroundColor: "rgb(220 252 231)", // green-100
                        color: "rgb(22 101 52)", // green-800
                      }
                    : isCurrent
                      ? {
                          backgroundColor: "rgb(219 234 254)", // blue-100
                          color: "rgb(30 64 175)", // blue-800
                        }
                      : {
                          backgroundColor: "rgb(243 244 246)", // gray-100
                          color: "rgb(156 163 175)", // gray-400
                        }
                }
              >
                {isCompleted ? "\u2713" : i + 1}
              </div>
              <span
                className="text-xs font-body"
                style={{
                  color: isCurrent
                    ? "rgb(30 64 175)" // blue-800
                    : isCompleted
                      ? "rgb(22 101 52)" // green-800
                      : "rgb(156 163 175)", // gray-400
                }}
              >
                {step.label}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
