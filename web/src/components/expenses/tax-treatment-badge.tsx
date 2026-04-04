"use client";

// ---------------------------------------------------------------------------
// TaxTreatmentBadge — compact badge showing tax treatment type.
// ---------------------------------------------------------------------------

interface TaxTreatmentBadgeProps {
  treatment: string; // input_gst | tds_applicable | none
}

const TREATMENTS: Record<
  string,
  { label: string; bg: string; text: string }
> = {
  input_gst: {
    label: "GST",
    bg: "rgb(219 234 254)", // blue-100
    text: "rgb(30 64 175)", // blue-800
  },
  tds_applicable: {
    label: "TDS",
    bg: "rgb(254 243 199)", // amber-100
    text: "rgb(146 64 14)", // amber-800
  },
  none: {
    label: "No Tax",
    bg: "rgb(243 244 246)", // gray-100
    text: "rgb(55 65 81)", // gray-700
  },
};

export function TaxTreatmentBadge({ treatment }: TaxTreatmentBadgeProps) {
  const config = TREATMENTS[treatment] ?? TREATMENTS.none;

  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-heading"
      style={{
        backgroundColor: config.bg,
        color: config.text,
      }}
    >
      {config.label}
    </span>
  );
}
