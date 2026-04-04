"use client";

// ---------------------------------------------------------------------------
// AssetStatusBadge — inventory-specific status badge with color mapping.
// ---------------------------------------------------------------------------

interface AssetStatusBadgeProps {
  status: string;
}

const STATUS_CONFIG: Record<string, { label: string; bg: string; text: string }> = {
  available: { label: "Available", bg: "bg-green-100", text: "text-green-700" },
  checked_out: { label: "Checked Out", bg: "bg-blue-100", text: "text-blue-700" },
  under_repair: { label: "Under Repair", bg: "bg-amber-100", text: "text-amber-700" },
  retired: { label: "Retired", bg: "bg-gray-100", text: "text-gray-600" },
};

const FALLBACK = { label: "Unknown", bg: "bg-gray-100", text: "text-gray-600" };

export function AssetStatusBadge({ status }: AssetStatusBadgeProps) {
  const config = STATUS_CONFIG[status] ?? FALLBACK;

  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium font-heading ${config.bg} ${config.text}`}
    >
      {config.label}
    </span>
  );
}
