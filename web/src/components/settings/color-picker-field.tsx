"use client";

import { useCallback, useState, useEffect } from "react";
import { RotateCcw } from "lucide-react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ColorPickerFieldProps {
  label: string;
  value: string;
  defaultValue: string;
  onChange: (color: string) => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const HEX_REGEX = /^#[0-9A-Fa-f]{6}$/;

function isValidHex(value: string): boolean {
  return HEX_REGEX.test(value);
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ColorPickerField({
  label,
  value,
  defaultValue,
  onChange,
}: ColorPickerFieldProps) {
  const [hexInput, setHexInput] = useState(value);
  const isModified = value.toLowerCase() !== defaultValue.toLowerCase();

  // Sync text input when the external value changes (e.g., reset)
  useEffect(() => {
    setHexInput(value);
  }, [value]);

  const handleHexChange = useCallback(
    (raw: string) => {
      // Always update the text field so the user can type freely
      setHexInput(raw);

      // Normalise: add # if missing
      const normalised = raw.startsWith("#") ? raw : `#${raw}`;
      if (isValidHex(normalised)) {
        onChange(normalised);
      }
    },
    [onChange],
  );

  const handleColorInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const hex = e.target.value;
      setHexInput(hex);
      onChange(hex);
    },
    [onChange],
  );

  const handleReset = useCallback(() => {
    onChange(defaultValue);
    setHexInput(defaultValue);
  }, [defaultValue, onChange]);

  return (
    <div className="flex items-center gap-3">
      {/* Color swatch + native picker */}
      <label className="relative flex-shrink-0 cursor-pointer">
        <span
          className="block h-9 w-9 rounded-lg border shadow-sm transition-colors duration-200"
          style={{
            backgroundColor: value,
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        />
        <input
          type="color"
          value={value}
          onChange={handleColorInputChange}
          className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
          aria-label={`Pick color for ${label}`}
        />
      </label>

      {/* Label + hex input */}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <span
            className="text-xs font-heading font-medium"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            {label}
          </span>
          {isModified && (
            <span
              className="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-heading font-semibold"
              style={{
                backgroundColor: "var(--thittam-accent, #f59e0b)",
                color: "var(--thittam-accent-foreground, #1c1917)",
              }}
            >
              Modified
            </span>
          )}
        </div>
        <input
          type="text"
          value={hexInput}
          onChange={(e) => handleHexChange(e.target.value)}
          maxLength={7}
          spellCheck={false}
          className="w-24 rounded border px-2 py-1 font-mono tabular-nums text-xs transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-offset-1"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            color: "var(--thittam-foreground, #0f172a)",
            backgroundColor: "var(--thittam-background, #fff)",
          }}
          aria-label={`Hex value for ${label}`}
        />
      </div>

      {/* Reset button */}
      {isModified && (
        <button
          type="button"
          onClick={handleReset}
          className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md transition-colors hover:opacity-80"
          style={{
            backgroundColor: "var(--thittam-muted, #f1f5f9)",
            color: "var(--thittam-muted-foreground, #64748b)",
          }}
          title="Reset to vertical default"
          aria-label={`Reset ${label} to default`}
        >
          <RotateCcw className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
}
