"use client";

import { useCallback, useRef, useState } from "react";
import { Upload, Trash2, ImageIcon } from "lucide-react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface LogoUploaderProps {
  label: string;
  currentUrl: string | null;
  onUpload: (file: File) => void;
  onRemove: () => void;
  maxSizeKB?: number;
  previewSize?: { width: number; height: number };
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const ACCEPTED_TYPES = ["image/png", "image/svg+xml", "image/jpeg"];
const ACCEPTED_EXTENSIONS = ".png,.svg,.jpg,.jpeg";

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function LogoUploader({
  label,
  currentUrl,
  onUpload,
  onRemove,
  maxSizeKB = 500,
  previewSize = { width: 160, height: 80 },
}: LogoUploaderProps) {
  const [error, setError] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const validateAndUpload = useCallback(
    (file: File) => {
      setError(null);

      if (!ACCEPTED_TYPES.includes(file.type)) {
        setError("Invalid file type. Accepted: PNG, SVG, JPG.");
        return;
      }

      if (file.size > maxSizeKB * 1024) {
        setError(`File too large. Maximum size: ${maxSizeKB}KB.`);
        return;
      }

      onUpload(file);
    },
    [maxSizeKB, onUpload],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragging(false);
      const file = e.dataTransfer.files[0];
      if (file) validateAndUpload(file);
    },
    [validateAndUpload],
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) validateAndUpload(file);
      // Reset so the same file can be re-selected
      if (fileInputRef.current) fileInputRef.current.value = "";
    },
    [validateAndUpload],
  );

  const handleRemove = useCallback(() => {
    setError(null);
    onRemove();
  }, [onRemove]);

  return (
    <div className="flex flex-col gap-2">
      <span
        className="text-xs font-heading font-medium"
        style={{ color: "var(--thittam-foreground, #0f172a)" }}
      >
        {label}
      </span>

      {currentUrl ? (
        /* -- Preview state -- */
        <div
          className="relative flex items-center justify-center rounded-xl border p-4 transition-colors duration-200"
          style={{
            borderColor: "var(--thittam-border, #e2e8f0)",
            backgroundColor: "var(--thittam-muted, #f1f5f9)",
            width: previewSize.width + 32,
            minHeight: previewSize.height + 32,
          }}
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={currentUrl}
            alt={label}
            className="object-contain"
            style={{
              maxWidth: previewSize.width,
              maxHeight: previewSize.height,
            }}
          />
          <button
            type="button"
            onClick={handleRemove}
            className="absolute -right-2 -top-2 flex h-6 w-6 items-center justify-center rounded-full bg-red-500 text-white shadow-sm transition-colors hover:bg-red-600"
            title={`Remove ${label.toLowerCase()}`}
            aria-label={`Remove ${label.toLowerCase()}`}
          >
            <Trash2 className="h-3 w-3" />
          </button>
        </div>
      ) : (
        /* -- Upload drop zone -- */
        <div
          role="button"
          tabIndex={0}
          onClick={() => fileInputRef.current?.click()}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              fileInputRef.current?.click();
            }
          }}
          onDrop={handleDrop}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          className="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed px-4 py-6 transition-colors duration-200"
          style={{
            borderColor: isDragging
              ? "var(--thittam-primary, #3b82f6)"
              : "var(--thittam-border, #e2e8f0)",
            backgroundColor: isDragging
              ? "var(--thittam-muted, #f1f5f9)"
              : "transparent",
          }}
          aria-label={`Upload ${label.toLowerCase()}`}
        >
          {isDragging ? (
            <Upload
              className="h-6 w-6"
              style={{ color: "var(--thittam-primary, #3b82f6)" }}
            />
          ) : (
            <ImageIcon
              className="h-6 w-6"
              style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
            />
          )}
          <span
            className="text-xs font-body text-center"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            {isDragging
              ? "Drop file here"
              : "Drag & drop or click to browse"}
          </span>
          <span
            className="text-[10px] font-mono tabular-nums"
            style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
          >
            PNG, SVG, JPG (max {maxSizeKB}KB)
          </span>
        </div>
      )}

      <input
        ref={fileInputRef}
        type="file"
        accept={ACCEPTED_EXTENSIONS}
        onChange={handleFileInput}
        className="hidden"
        aria-hidden="true"
      />

      {error && (
        <span className="text-xs font-body text-red-600">{error}</span>
      )}
    </div>
  );
}
