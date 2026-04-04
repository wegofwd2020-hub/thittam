// ---------------------------------------------------------------------------
// Skeleton loading states — pulse-animated placeholders.
// ---------------------------------------------------------------------------

function Bone({ className = "" }: { className?: string }) {
  return (
    <div
      className={`animate-pulse rounded-md ${className}`}
      style={{ backgroundColor: "var(--thittam-muted, #e2e8f0)" }}
    />
  );
}

// ---------------------------------------------------------------------------
// TableSkeleton — matches DataTable layout
// ---------------------------------------------------------------------------

interface TableSkeletonProps {
  rows?: number;
  cols?: number;
}

export function TableSkeleton({ rows = 5, cols = 4 }: TableSkeletonProps) {
  return (
    <div
      className="overflow-hidden rounded-lg border"
      style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
    >
      {/* Header row */}
      <div
        className="flex gap-4 px-4 py-3"
        style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
      >
        {Array.from({ length: cols }).map((_, i) => (
          <Bone key={i} className="h-3 flex-1" />
        ))}
      </div>
      {/* Body rows */}
      {Array.from({ length: rows }).map((_, rowIdx) => (
        <div
          key={rowIdx}
          className="flex gap-4 px-4 py-4"
          style={{
            borderTop: "1px solid var(--thittam-border, #e2e8f0)",
          }}
        >
          {Array.from({ length: cols }).map((_, colIdx) => (
            <Bone
              key={colIdx}
              className="h-3 flex-1"
              // Vary widths slightly for a more natural look
            />
          ))}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// CardSkeleton — single card placeholder
// ---------------------------------------------------------------------------

export function CardSkeleton() {
  return (
    <div
      className="rounded-lg border p-5"
      style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
    >
      <div className="flex items-center justify-between">
        <Bone className="h-4 w-24" />
        <Bone className="h-5 w-16 rounded-full" />
      </div>
      <Bone className="mt-4 h-7 w-32" />
      <Bone className="mt-3 h-3 w-full" />
      <Bone className="mt-2 h-3 w-3/4" />
    </div>
  );
}

// ---------------------------------------------------------------------------
// PageSkeleton — full page placeholder (header + cards + table)
// ---------------------------------------------------------------------------

export function PageSkeleton() {
  return (
    <div className="space-y-6">
      {/* Page header skeleton */}
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-3">
          <Bone className="h-10 w-10 rounded-lg" />
          <div>
            <Bone className="h-6 w-48" />
            <Bone className="mt-2 h-3 w-72" />
          </div>
        </div>
        <Bone className="h-9 w-28 rounded-lg" />
      </div>

      {/* Summary cards row */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <CardSkeleton key={i} />
        ))}
      </div>

      {/* Table skeleton */}
      <TableSkeleton rows={5} cols={5} />
    </div>
  );
}
