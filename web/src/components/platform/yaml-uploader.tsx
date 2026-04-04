"use client";

import { useState, useRef, useCallback } from "react";
import {
  Upload,
  FileText,
  CheckCircle2,
  XCircle,
  Loader2,
  X,
} from "lucide-react";

// ---------------------------------------------------------------------------
// YAMLUploader — drag-and-drop YAML file upload with mock validation.
// Simulates the thittam-cli validate output for vertical definition files.
// ---------------------------------------------------------------------------

interface ValidationResult {
  valid: boolean;
  errors?: string[];
  summary?: {
    name: string;
    version: string;
    sections: { name: string; ok: boolean; detail?: string }[];
  };
}

interface YAMLUploaderProps {
  onValidated: (result: ValidationResult) => void;
}

// Mock validation — simulates what thittam-cli validate does
function mockValidateYAML(fileName: string): Promise<ValidationResult> {
  return new Promise((resolve) => {
    setTimeout(() => {
      // Simulate a failure for files containing "invalid"
      if (fileName.toLowerCase().includes("invalid")) {
        resolve({
          valid: false,
          errors: [
            "missing required field: metadata.version",
            "entity_labels.project must be a non-empty string",
            "budget_categories must contain at least 1 category",
          ],
        });
        return;
      }

      resolve({
        valid: true,
        summary: {
          name: fileName.replace(/\.ya?ml$/i, "").replace(/-/g, " "),
          version: "1.0.0",
          sections: [
            { name: "Metadata", ok: true },
            { name: "Entity Labels", ok: true },
            { name: "Phase Types", ok: true },
            { name: "Budget Categories", ok: true },
            { name: "Expense Workflows", ok: true },
            { name: "Inventory Categories", ok: true },
            { name: "Chart of Accounts", ok: true },
            { name: "Custom Fields", ok: true },
            { name: "Theme Config", ok: true },
          ],
        },
      });
    }, 1500);
  });
}

export function YAMLUploader({ onValidated }: YAMLUploaderProps) {
  const [isDragging, setIsDragging] = useState(false);
  const [fileName, setFileName] = useState<string | null>(null);
  const [isValidating, setIsValidating] = useState(false);
  const [result, setResult] = useState<ValidationResult | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFile = useCallback(
    async (file: File) => {
      setFileName(file.name);
      setResult(null);
      setIsValidating(true);

      const validationResult = await mockValidateYAML(file.name);
      setResult(validationResult);
      setIsValidating(false);
      onValidated(validationResult);
    },
    [onValidated],
  );

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setIsDragging(false);
    const file = e.dataTransfer.files[0];
    if (file && /\.ya?ml$/i.test(file.name)) {
      handleFile(file);
    }
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault();
    setIsDragging(true);
  }

  function handleDragLeave() {
    setIsDragging(false);
  }

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
  }

  function handleReset() {
    setFileName(null);
    setResult(null);
    setIsValidating(false);
    if (inputRef.current) inputRef.current.value = "";
  }

  return (
    <div className="space-y-4">
      {/* Drop zone */}
      <div
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onClick={() => inputRef.current?.click()}
        className={`flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed px-6 py-10 transition-colors ${
          isDragging
            ? "border-orange-400 bg-orange-50"
            : "border-slate-300 bg-slate-50 hover:border-slate-400 hover:bg-slate-100"
        }`}
      >
        <Upload
          className={`mb-3 h-8 w-8 ${
            isDragging ? "text-orange-500" : "text-slate-400"
          }`}
        />
        <p className="font-heading text-sm font-medium text-slate-700">
          Drag and drop a YAML file here
        </p>
        <p className="mt-1 font-body text-xs text-slate-500">
          or click to browse (.yaml, .yml)
        </p>
        <input
          ref={inputRef}
          type="file"
          accept=".yaml,.yml"
          onChange={handleInputChange}
          className="hidden"
        />
      </div>

      {/* File info + validation results */}
      {fileName && (
        <div className="rounded-xl border border-slate-200 bg-white">
          {/* File header */}
          <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
            <div className="flex items-center gap-2">
              <FileText className="h-4 w-4 text-slate-500" />
              <span className="font-mono text-sm text-slate-800">{fileName}</span>
            </div>
            <button
              onClick={handleReset}
              className="flex h-6 w-6 items-center justify-center rounded text-slate-400 hover:text-slate-600"
              aria-label="Remove file"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {/* Validating spinner */}
          {isValidating && (
            <div className="flex items-center gap-2 px-4 py-6">
              <Loader2 className="h-5 w-5 animate-spin text-orange-500" />
              <span className="font-body text-sm text-slate-600">
                Validating vertical definition...
              </span>
            </div>
          )}

          {/* Validation results */}
          {result && !isValidating && (
            <div className="px-4 py-4">
              {result.valid && result.summary ? (
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <CheckCircle2 className="h-5 w-5 text-green-600" />
                    <span className="font-heading text-sm font-semibold text-green-700">
                      Validation passed
                    </span>
                  </div>
                  <div className="space-y-1.5">
                    {result.summary.sections.map((section) => (
                      <div
                        key={section.name}
                        className="flex items-center gap-2"
                      >
                        <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />
                        <span className="font-body text-sm text-slate-700">
                          {section.name}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <XCircle className="h-5 w-5 text-red-600" />
                    <span className="font-heading text-sm font-semibold text-red-700">
                      Validation failed
                    </span>
                  </div>
                  <ul className="space-y-1.5 pl-1">
                    {result.errors?.map((err, i) => (
                      <li
                        key={i}
                        className="flex items-start gap-2 font-mono text-sm text-red-600"
                      >
                        <XCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                        {err}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
