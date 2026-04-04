"use client";

// ---------------------------------------------------------------------------
// DepreciationBadge — shows depreciation info for an inventory category.
// ---------------------------------------------------------------------------

interface DepreciationBadgeProps {
  depreciationYears: number | null;
}

export function DepreciationBadge({ depreciationYears }: DepreciationBadgeProps) {
  if (depreciationYears != null && depreciationYears > 0) {
    return (
      <span className="inline-flex items-center rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-medium font-heading text-blue-700">
        {depreciationYears}-year depreciation
      </span>
    );
  }

  return (
    <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-medium font-heading text-gray-600">
      No depreciation
    </span>
  );
}
