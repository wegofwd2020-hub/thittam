"use client";

import {
  Clapperboard,
  FolderKanban,
  Building2,
  CalendarDays,
  Users,
  Briefcase,
} from "lucide-react";

// ---------------------------------------------------------------------------
// VerticalCard — large selection card for the vertical picker.
// ---------------------------------------------------------------------------

interface VerticalCardProps {
  verticalId: string;
  name: string;
  description: string;
  companyName: string;
  userCount: number;
  projectCount: number;
  color: string;
  icon: string;
  isSelected: boolean;
  onSelect: () => void;
}

const ICON_MAP: Record<string, React.ComponentType<{ className?: string }>> = {
  Clapperboard,
  FolderKanban,
  Building2,
  CalendarDays,
  Users,
  Briefcase,
};

export function VerticalCard({
  name,
  description,
  companyName,
  userCount,
  projectCount,
  color,
  icon,
  isSelected,
  onSelect,
}: VerticalCardProps) {
  const Icon = ICON_MAP[icon] ?? Briefcase;

  return (
    <button
      type="button"
      onClick={onSelect}
      className={`relative flex w-full flex-col overflow-hidden rounded-xl border-2 bg-white text-left transition-all hover:shadow-lg ${
        isSelected
          ? "ring-2 ring-offset-2 shadow-lg"
          : "border-gray-200 hover:border-gray-300"
      }`}
      style={{
        borderColor: isSelected ? color : undefined,
      }}
    >
      {/* Color stripe */}
      <div className="h-2 w-full" style={{ backgroundColor: color }} />

      <div className="flex flex-1 flex-col p-6">
        {/* Icon */}
        <div
          className="mb-4 flex h-14 w-14 items-center justify-center rounded-xl"
          style={{ backgroundColor: `${color}15` }}
        >
          <span style={{ color }}><Icon className="h-7 w-7" /></span>
        </div>

        {/* Name */}
        <h3 className="text-lg font-semibold font-heading" style={{ color: "#0f172a" }}>
          {name}
        </h3>

        {/* Description */}
        <p className="mt-1 text-sm font-body" style={{ color: "#64748b" }}>
          {description}
        </p>

        {/* Example company */}
        <p className="mt-3 text-xs font-heading" style={{ color: "#94a3b8" }}>
          e.g. {companyName}
        </p>

        {/* Stats */}
        <div className="mt-4 flex items-center gap-3">
          <span className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-1 text-xs font-mono tabular-nums">
            <Users className="h-3 w-3" style={{ color: "#64748b" }} />
            {userCount} users
          </span>
          <span className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-1 text-xs font-mono tabular-nums">
            <Briefcase className="h-3 w-3" style={{ color: "#64748b" }} />
            {projectCount} projects
          </span>
        </div>
      </div>

      {/* Selected indicator */}
      {isSelected && (
        <div
          className="absolute right-3 top-5 flex h-6 w-6 items-center justify-center rounded-full text-white"
          style={{ backgroundColor: color }}
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        </div>
      )}
    </button>
  );
}
