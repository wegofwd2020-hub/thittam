"use client";

// ---------------------------------------------------------------------------
// DualApprovalIndicator — shows dual-approval status for high-value expenses.
// ---------------------------------------------------------------------------

interface DualApprovalIndicatorProps {
  amount: string;
  threshold: string;
  approvals: { role: string; approvedAt: string | null }[];
}

function parseDecimal(value: string): number {
  const num = parseFloat(value);
  return isNaN(num) ? 0 : num;
}

function formatAmount(num: number): string {
  return `$${num.toLocaleString("en-US", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function DualApprovalIndicator({
  amount,
  threshold,
  approvals,
}: DualApprovalIndicatorProps) {
  const amountNum = parseDecimal(amount);
  const thresholdNum = parseDecimal(threshold);
  const requiresDual = amountNum > thresholdNum;

  if (!requiresDual) return null;

  return (
    <div
      className="rounded-md border px-4 py-3 space-y-3"
      style={{
        borderColor: "rgb(253 230 138)", // amber-200
        backgroundColor: "rgb(254 252 232)", // amber-50
      }}
    >
      {/* Header */}
      <div className="flex items-center gap-2">
        <span
          className="text-sm font-heading"
          style={{ color: "rgb(146 64 14)" }} // amber-800
        >
          Dual Approval Required
        </span>
        <span
          className="text-xs font-body"
          style={{ color: "rgb(180 83 9)" }} // amber-700
        >
          (above{" "}
          <span className="font-mono tabular-nums">
            {formatAmount(thresholdNum)}
          </span>
          )
        </span>
      </div>

      {/* Approval slots */}
      <div className="flex items-center gap-4">
        {approvals.slice(0, 2).map((approval, i) => {
          const isApproved = approval.approvedAt !== null;

          return (
            <div key={i} className="flex items-center gap-2">
              {/* Status circle */}
              <div
                className="flex h-5 w-5 items-center justify-center rounded-full text-xs"
                style={
                  isApproved
                    ? {
                        backgroundColor: "rgb(220 252 231)", // green-100
                        color: "rgb(22 101 52)", // green-800
                      }
                    : {
                        backgroundColor: "rgb(243 244 246)", // gray-100
                        color: "rgb(156 163 175)", // gray-400
                      }
                }
              >
                {isApproved ? "\u2713" : "\u25CB"}
              </div>

              {/* Role + timestamp */}
              <div>
                <span
                  className="text-xs font-heading"
                  style={{
                    color: isApproved
                      ? "rgb(22 101 52)" // green-800
                      : "rgb(107 114 128)", // gray-500
                  }}
                >
                  {approval.role}
                </span>
                {isApproved && approval.approvedAt && (
                  <span
                    className="ml-1 text-xs font-mono tabular-nums"
                    style={{ color: "rgb(107 114 128)" }} // gray-500
                  >
                    {new Date(approval.approvedAt).toLocaleDateString()}
                  </span>
                )}
              </div>
            </div>
          );
        })}

        {/* If fewer than 2 approval slots provided, show empty pending slots */}
        {approvals.length < 2 &&
          Array.from({ length: 2 - approvals.length }).map((_, i) => (
            <div key={`empty-${i}`} className="flex items-center gap-2">
              <div
                className="flex h-5 w-5 items-center justify-center rounded-full text-xs"
                style={{
                  backgroundColor: "rgb(243 244 246)", // gray-100
                  color: "rgb(156 163 175)", // gray-400
                }}
              >
                &#9675;
              </div>
              <span
                className="text-xs font-heading"
                style={{ color: "rgb(156 163 175)" }} // gray-400
              >
                Pending
              </span>
            </div>
          ))}
      </div>
    </div>
  );
}
