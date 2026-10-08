"use client";

// ---------------------------------------------------------------------------
// TaxTreatmentBadge — compact badge showing tax treatment type.
// ---------------------------------------------------------------------------

interface TaxTreatmentBadgeProps {
  treatment: string; // see validTaxTreatments in pkg/vertical/validator.go
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
  us_1099_nec: {
    label: "1099-NEC",
    bg: "rgb(237 233 254)", // violet-100
    text: "rgb(91 33 182)", // violet-800
  },
  us_sales_tax_paid: {
    label: "Sales Tax",
    bg: "rgb(220 252 231)", // green-100
    text: "rgb(22 101 52)", // green-800
  },
  us_use_tax: {
    label: "Use Tax",
    bg: "rgb(255 237 213)", // orange-100
    text: "rgb(154 52 18)", // orange-800
  },
  us_meals_50pct: {
    label: "Meals 50%",
    bg: "rgb(252 231 243)", // pink-100
    text: "rgb(157 23 77)", // pink-800
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
