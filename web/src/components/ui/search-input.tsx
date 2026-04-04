"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Search, X } from "lucide-react";

// ---------------------------------------------------------------------------
// SearchInput — debounced search field with clear button.
// ---------------------------------------------------------------------------

interface SearchInputProps {
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
  debounceMs?: number;
}

export function SearchInput({
  placeholder = "Search\u2026",
  value,
  onChange,
  debounceMs = 300,
}: SearchInputProps) {
  const [local, setLocal] = useState(value);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Sync external value changes into local state
  useEffect(() => {
    setLocal(value);
  }, [value]);

  const emitChange = useCallback(
    (next: string) => {
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        onChange(next);
      }, debounceMs);
    },
    [onChange, debounceMs],
  );

  // Cleanup timer on unmount
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const next = e.target.value;
    setLocal(next);
    emitChange(next);
  }

  function handleClear() {
    setLocal("");
    onChange("");
    if (timerRef.current) clearTimeout(timerRef.current);
  }

  return (
    <div className="relative">
      <Search
        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
        style={{ color: "var(--thittam-muted-foreground, #94a3b8)" }}
        aria-hidden="true"
      />
      <input
        type="text"
        value={local}
        onChange={handleChange}
        placeholder={placeholder}
        className="h-9 w-full rounded-lg border py-2 pl-9 pr-8 text-sm font-body outline-none transition-colors focus:ring-2"
        style={{
          borderColor: "var(--thittam-border, #e2e8f0)",
          backgroundColor: "var(--thittam-background, #fff)",
          color: "var(--thittam-foreground, #0f172a)",
          // ring color on focus handled via Tailwind ring-[color], fallback:
        }}
      />
      {local.length > 0 && (
        <button
          type="button"
          onClick={handleClear}
          className="absolute right-2 top-1/2 -translate-y-1/2 rounded-sm p-0.5 opacity-60 hover:opacity-100"
          aria-label="Clear search"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
}
