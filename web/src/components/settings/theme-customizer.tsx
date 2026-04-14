"use client";

import { useCallback, useMemo, useState } from "react";
import { RotateCcw, Save, Eye } from "lucide-react";
import { toast } from "sonner";

import { useTheme } from "@/lib/themes/provider";
import { getVerticalTheme } from "@/lib/themes/verticals";
import { THEME_PRESETS } from "@/lib/themes/presets";
import { useAccessibility } from "@/lib/accessibility/dyslexia-provider";
import { useSaveThemeOverride, useResetThemeOverride } from "@/lib/hooks/use-theme-admin";

import type { ThemeColors, SidebarColors, StatusColors } from "@/lib/themes/types";
import type { ThemeOverride } from "@/lib/api/theme";

import { ColorPickerField } from "./color-picker-field";
import { LogoUploader } from "./logo-uploader";
import { ThemePreviewPanel } from "./theme-preview-panel";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Build a full ThemeColors by merging partial overrides onto the default. */
function mergeColors(
  base: ThemeColors,
  partial: Partial<ThemeColors> | undefined,
): ThemeColors {
  if (!partial) return { ...base };

  return {
    ...base,
    ...partial,
    sidebar: {
      ...base.sidebar,
      ...(partial.sidebar as Partial<SidebarColors> | undefined),
    },
    status: {
      ...base.status,
      ...(partial.status as Partial<StatusColors> | undefined),
    },
  };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ThemeCustomizer() {
  const { theme, setThemeOverride } = useTheme();
  const { dyslexiaMode, toggleDyslexia } = useAccessibility();

  const saveThemeMutation = useSaveThemeOverride();
  const resetThemeMutation = useResetThemeOverride();

  // The vertical's unmodified default theme
  const defaultTheme = useMemo(
    () => getVerticalTheme(theme.id) ?? theme,
    [theme],
  );
  const defaultColors = defaultTheme.colors;

  // ── Local state for color overrides (not yet committed to API) ────────
  const [colorOverrides, setColorOverrides] = useState<Partial<ThemeColors>>({});
  const [brandingState, setBrandingState] = useState<{
    logo: string | null;
    favicon: string | null;
    loginBg: string;
  }>({
    logo: theme.branding.logo ?? null,
    favicon: theme.branding.favicon ?? null,
    loginBg: theme.branding.loginBg ?? "",
  });

  // Resolved colors = defaults + local overrides (for preview)
  const resolvedColors = useMemo(
    () => mergeColors(defaultColors, colorOverrides),
    [defaultColors, colorOverrides],
  );

  // ── Color change helpers ──────────────────────────────────────────────

  const setColor = useCallback(
    (key: keyof ThemeColors, value: string) => {
      setColorOverrides((prev) => ({ ...prev, [key]: value }));
    },
    [],
  );

  const setSidebarColor = useCallback(
    (key: keyof SidebarColors, value: string) => {
      setColorOverrides((prev) => ({
        ...prev,
        sidebar: { ...defaultColors.sidebar, ...prev.sidebar, [key]: value },
      }));
    },
    [defaultColors.sidebar],
  );

  const setStatusColor = useCallback(
    (key: keyof StatusColors, value: string) => {
      setColorOverrides((prev) => ({
        ...prev,
        status: { ...defaultColors.status, ...prev.status, [key]: value },
      }));
    },
    [defaultColors.status],
  );

  const resetColor = useCallback(
    (key: keyof ThemeColors) => {
      setColorOverrides((prev) => {
        const next = { ...prev };
        delete next[key];
        return next;
      });
    },
    [],
  );

  // ── Actions ───────────────────────────────────────────────────────────

  const handlePreview = useCallback(() => {
    setThemeOverride({ colors: resolvedColors });
    toast.info("Preview applied (not saved)");
  }, [resolvedColors, setThemeOverride]);

  const handleSave = useCallback(() => {
    const override: ThemeOverride = {};

    // Only include colors that differ from defaults
    const hasColorChanges = Object.keys(colorOverrides).length > 0;
    if (hasColorChanges) {
      override.colors = colorOverrides;
    }

    const hasBrandingChanges =
      brandingState.logo !== (defaultTheme.branding.logo ?? null) ||
      brandingState.favicon !== (defaultTheme.branding.favicon ?? null) ||
      brandingState.loginBg !== (defaultTheme.branding.loginBg ?? "");

    if (hasBrandingChanges) {
      override.branding = {
        logo: brandingState.logo ?? undefined,
        favicon: brandingState.favicon ?? undefined,
        loginBg: brandingState.loginBg || undefined,
      };
    }

    // Apply to the live theme
    setThemeOverride({ colors: resolvedColors });

    // Persist to API
    saveThemeMutation.mutate(override);

    // Also persist to localStorage for immediate load on next visit
    try {
      localStorage.setItem("thittam-theme-override", JSON.stringify(override));
    } catch {
      // localStorage may be full or unavailable
    }
  }, [
    colorOverrides,
    brandingState,
    defaultTheme.branding,
    resolvedColors,
    setThemeOverride,
    saveThemeMutation,
  ]);

  const handleResetAll = useCallback(() => {
    if (!window.confirm("Reset all theme customizations to the vertical default? This cannot be undone.")) {
      return;
    }

    setColorOverrides({});
    setBrandingState({ logo: null, favicon: null, loginBg: "" });
    setThemeOverride(null);
    resetThemeMutation.mutate();

    try {
      localStorage.removeItem("thittam-theme-override");
    } catch {
      // ignore
    }
  }, [setThemeOverride, resetThemeMutation]);

  // ── Logo handlers ─────────────────────────────────────────────────────

  const handleLogoUpload = useCallback((file: File) => {
    // Create a local preview URL; real upload happens on Save
    const url = URL.createObjectURL(file);
    setBrandingState((prev) => ({ ...prev, logo: url }));
  }, []);

  const handleFaviconUpload = useCallback((file: File) => {
    const url = URL.createObjectURL(file);
    setBrandingState((prev) => ({ ...prev, favicon: url }));
  }, []);

  // ── Render ────────────────────────────────────────────────────────────

  const applyPreset = useCallback(
    (presetId: string) => {
      const preset = THEME_PRESETS.find((p) => p.id === presetId);
      if (!preset) return;
      setColorOverrides(preset.colors);
      setThemeOverride({ colors: mergeColors(defaultColors, preset.colors) });
      toast.info(`Preset "${preset.name}" applied (not saved)`);
    },
    [defaultColors, setThemeOverride],
  );

  return (
    <div className="flex gap-8">
      {/* ================================================================= */}
      {/* Left column — Configuration form (60%)                            */}
      {/* ================================================================= */}
      <div className="flex w-[60%] min-w-0 flex-col gap-8">

        {/* ── Theme Presets ──────────────────────────────────────────── */}
        <section
          className="rounded-xl border p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <h3
            className="mb-3 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Presets
          </h3>
          <div className="grid gap-3 sm:grid-cols-3">
            {THEME_PRESETS.map((preset) => (
              <button
                key={preset.id}
                type="button"
                onClick={() => applyPreset(preset.id)}
                className="flex flex-col items-start gap-2 rounded-lg border p-3 text-left transition-colors hover:opacity-90"
                style={{
                  borderColor: "var(--thittam-border, #e2e8f0)",
                }}
              >
                <div className="flex gap-1">
                  <span
                    className="h-5 w-5 rounded-full border"
                    style={{
                      backgroundColor: preset.colors.primary ?? defaultColors.primary,
                      borderColor: "var(--thittam-border, #e2e8f0)",
                    }}
                  />
                  <span
                    className="h-5 w-5 rounded-full border"
                    style={{
                      backgroundColor: preset.colors.accent ?? defaultColors.accent,
                      borderColor: "var(--thittam-border, #e2e8f0)",
                    }}
                  />
                </div>
                <span
                  className="font-heading text-sm font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  {preset.name}
                </span>
                <span
                  className="font-body text-xs"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  {preset.description}
                </span>
              </button>
            ))}
          </div>
        </section>

        {/* ── Base Theme ─────────────────────────────────────────────── */}
        <section
          className="rounded-xl border p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <h3
            className="mb-4 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Base Theme
          </h3>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Current Vertical
              </span>
              <span
                className="mt-1 block font-heading text-sm font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {defaultTheme.name}
              </span>
            </div>
            <div>
              <span
                className="block text-xs font-heading font-medium"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Theme ID
              </span>
              <span
                className="mt-1 block font-mono tabular-nums text-sm"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {defaultTheme.id}
              </span>
            </div>
          </div>
        </section>

        {/* ── Color Overrides ────────────────────────────────────────── */}
        <section
          className="rounded-xl border p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <h3
            className="mb-4 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Color Overrides
          </h3>

          <div className="flex flex-col gap-5">
            {/* Core colors */}
            <div className="flex flex-col gap-3">
              <span
                className="text-[10px] font-heading font-semibold uppercase tracking-wider"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Core Colors
              </span>
              <div className="grid gap-4 sm:grid-cols-2">
                <ColorPickerField
                  label="Primary Color"
                  value={resolvedColors.primary}
                  defaultValue={defaultColors.primary}
                  onChange={(v) => setColor("primary", v)}
                />
                <ColorPickerField
                  label="Accent Color"
                  value={resolvedColors.accent}
                  defaultValue={defaultColors.accent}
                  onChange={(v) => setColor("accent", v)}
                />
              </div>
            </div>

            {/* Sidebar colors */}
            <div className="flex flex-col gap-3">
              <span
                className="text-[10px] font-heading font-semibold uppercase tracking-wider"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Sidebar
              </span>
              <div className="grid gap-4 sm:grid-cols-2">
                <ColorPickerField
                  label="Sidebar Background"
                  value={resolvedColors.sidebar.bg}
                  defaultValue={defaultColors.sidebar.bg}
                  onChange={(v) => setSidebarColor("bg", v)}
                />
                <ColorPickerField
                  label="Sidebar Active Item"
                  value={resolvedColors.sidebar.activeItem}
                  defaultValue={defaultColors.sidebar.activeItem}
                  onChange={(v) => setSidebarColor("activeItem", v)}
                />
              </div>
            </div>

            {/* Status colors */}
            <div className="flex flex-col gap-3">
              <span
                className="text-[10px] font-heading font-semibold uppercase tracking-wider"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Status Colors
              </span>
              <div className="grid gap-4 sm:grid-cols-3">
                <ColorPickerField
                  label="On Track"
                  value={resolvedColors.status.onTrack}
                  defaultValue={defaultColors.status.onTrack}
                  onChange={(v) => setStatusColor("onTrack", v)}
                />
                <ColorPickerField
                  label="At Risk"
                  value={resolvedColors.status.atRisk}
                  defaultValue={defaultColors.status.atRisk}
                  onChange={(v) => setStatusColor("atRisk", v)}
                />
                <ColorPickerField
                  label="Over Budget"
                  value={resolvedColors.status.overBudget}
                  defaultValue={defaultColors.status.overBudget}
                  onChange={(v) => setStatusColor("overBudget", v)}
                />
              </div>
            </div>
          </div>
        </section>

        {/* ── Logo & Branding ────────────────────────────────────────── */}
        <section
          className="rounded-xl border p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <h3
            className="mb-4 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Logo & Branding
          </h3>

          <div className="grid gap-6 sm:grid-cols-2">
            <LogoUploader
              label="Company Logo"
              currentUrl={brandingState.logo}
              onUpload={handleLogoUpload}
              onRemove={() =>
                setBrandingState((prev) => ({ ...prev, logo: null }))
              }
              maxSizeKB={500}
              previewSize={{ width: 160, height: 80 }}
            />
            <LogoUploader
              label="Favicon"
              currentUrl={brandingState.favicon}
              onUpload={handleFaviconUpload}
              onRemove={() =>
                setBrandingState((prev) => ({ ...prev, favicon: null }))
              }
              maxSizeKB={100}
              previewSize={{ width: 48, height: 48 }}
            />
          </div>

          {/* Login Background URL */}
          <div className="mt-5">
            <label
              className="block text-xs font-heading font-medium"
              style={{ color: "var(--thittam-foreground, #0f172a)" }}
            >
              Login Background URL
            </label>
            <input
              type="url"
              value={brandingState.loginBg}
              onChange={(e) =>
                setBrandingState((prev) => ({ ...prev, loginBg: e.target.value }))
              }
              placeholder="https://example.com/background.jpg"
              className="mt-1 w-full rounded-lg border px-3 py-2 font-mono tabular-nums text-xs transition-colors focus:outline-none focus:ring-2 focus:ring-offset-1"
              style={{
                borderColor: "var(--thittam-border, #e2e8f0)",
                color: "var(--thittam-foreground, #0f172a)",
                backgroundColor: "var(--thittam-background, #fff)",
              }}
            />
          </div>
        </section>

        {/* ── Typography Preview ──────────────────────────────────────── */}
        <section
          className="rounded-xl border p-5"
          style={{
            backgroundColor: "var(--thittam-background, #fff)",
            borderColor: "var(--thittam-border, #e2e8f0)",
          }}
        >
          <h3
            className="mb-4 font-heading text-sm font-semibold"
            style={{ color: "var(--thittam-foreground, #0f172a)" }}
          >
            Typography Preview
          </h3>

          <div className="flex flex-col gap-4">
            <div
              className="rounded-lg p-4"
              style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
            >
              <span
                className="block text-[10px] font-heading font-semibold uppercase tracking-wider"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Heading Font (Inter)
              </span>
              <span
                className="mt-1 block font-heading text-lg font-bold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Heading Text Sample
              </span>
            </div>

            <div
              className="rounded-lg p-4"
              style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
            >
              <span
                className="block text-[10px] font-heading font-semibold uppercase tracking-wider"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Body Font (Merriweather)
              </span>
              <span
                className="mt-1 block font-body text-sm leading-relaxed"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                Body text sample paragraph. This font is used for descriptions,
                tooltips, paragraphs, and form input text throughout the
                application.
              </span>
            </div>

            <div
              className="rounded-lg p-4"
              style={{ backgroundColor: "var(--thittam-muted, #f1f5f9)" }}
            >
              <span
                className="block text-[10px] font-heading font-semibold uppercase tracking-wider"
                style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
              >
                Mono Font (JetBrains Mono)
              </span>
              <span
                className="mt-1 block font-mono tabular-nums text-lg font-semibold"
                style={{ color: "var(--thittam-foreground, #0f172a)" }}
              >
                {"\u20B9"}12,50,000.00 &mdash; ACC-5100
              </span>
            </div>

            {/* Dyslexia mode toggle */}
            <div
              className="flex items-center justify-between rounded-lg border px-4 py-3"
              style={{ borderColor: "var(--thittam-border, #e2e8f0)" }}
            >
              <div>
                <span
                  className="block text-xs font-heading font-medium"
                  style={{ color: "var(--thittam-foreground, #0f172a)" }}
                >
                  Dyslexia Mode
                </span>
                <span
                  className="block text-[10px] font-body"
                  style={{ color: "var(--thittam-muted-foreground, #64748b)" }}
                >
                  Switches all fonts to OpenDyslexic with increased spacing
                </span>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={dyslexiaMode}
                onClick={toggleDyslexia}
                className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full transition-colors ${
                  dyslexiaMode ? "" : "bg-gray-200"
                }`}
                style={
                  dyslexiaMode
                    ? { backgroundColor: "var(--thittam-primary, #3b82f6)" }
                    : {}
                }
              >
                <span
                  className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-sm ring-0 transition-transform ${
                    dyslexiaMode ? "translate-x-5" : "translate-x-0.5"
                  } mt-0.5`}
                />
              </button>
            </div>
          </div>
        </section>

        {/* ── Action Buttons ─────────────────────────────────────────── */}
        <div className="flex items-center gap-3 pb-8">
          <button
            type="button"
            onClick={handlePreview}
            className="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-heading font-medium transition-colors hover:opacity-90"
            style={{
              borderColor: "var(--thittam-border, #e2e8f0)",
              color: "var(--thittam-foreground, #0f172a)",
              backgroundColor: "var(--thittam-background, #fff)",
            }}
          >
            <Eye className="h-4 w-4" />
            Preview
          </button>

          <button
            type="button"
            onClick={handleSave}
            disabled={saveThemeMutation.isPending}
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
            style={{ backgroundColor: "var(--thittam-primary, #3b82f6)" }}
          >
            <Save className="h-4 w-4" />
            {saveThemeMutation.isPending ? "Saving..." : "Save Theme"}
          </button>

          <button
            type="button"
            onClick={handleResetAll}
            disabled={resetThemeMutation.isPending}
            className="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-heading font-medium text-red-600 transition-colors hover:bg-red-50 disabled:opacity-50"
          >
            <RotateCcw className="h-4 w-4" />
            Reset All to Default
          </button>
        </div>
      </div>

      {/* ================================================================= */}
      {/* Right column — Live preview (40%)                                 */}
      {/* ================================================================= */}
      <div className="w-[40%]">
        <ThemePreviewPanel colors={resolvedColors} />
      </div>
    </div>
  );
}
