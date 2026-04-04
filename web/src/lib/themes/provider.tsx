"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { EntityLabels, ThittamTheme } from "./types";
import { getVerticalDefinition } from "./verticals";
import { themeToCssVars } from "./css-vars";

// ---------------------------------------------------------------------------
// Deep merge helper — merges `override` into `base`, returning a new object.
// Only plain objects are recursed; primitives / arrays are replaced outright.
// ---------------------------------------------------------------------------
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- recursive merge needs flexible typing
function deepMerge(base: Record<string, any>, override: Record<string, any>): Record<string, any> {
  const result: Record<string, unknown> = { ...base };

  for (const key of Object.keys(override)) {
    const baseVal = result[key];
    const overrideVal = override[key];

    if (
      baseVal !== null &&
      overrideVal !== null &&
      typeof baseVal === "object" &&
      typeof overrideVal === "object" &&
      !Array.isArray(baseVal) &&
      !Array.isArray(overrideVal)
    ) {
      result[key] = deepMerge(
        baseVal as Record<string, unknown>,
        overrideVal as Record<string, unknown>,
      );
    } else if (overrideVal !== undefined) {
      result[key] = overrideVal;
    }
  }

  return result;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------
interface ThemeContextValue {
  theme: ThittamTheme;
  entityLabels: EntityLabels;
  setThemeOverride: (override: Partial<ThittamTheme> | null) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------
interface ThemeProviderProps {
  children: ReactNode;
  verticalId: string;
  themeOverride?: Partial<ThittamTheme> | null;
}

export function ThemeProvider({
  children,
  verticalId,
  themeOverride: initialOverride = null,
}: ThemeProviderProps) {
  const [override, setOverride] = useState<Partial<ThittamTheme> | null>(
    initialOverride ?? null,
  );

  const vertical = useMemo(
    () => getVerticalDefinition(verticalId),
    [verticalId],
  );

  // Fallback vertical so the app never renders without a theme
  const fallbackVertical = useMemo(
    () =>
      getVerticalDefinition("movie-production") ?? {
        theme: {} as ThittamTheme,
        entityLabels: {} as EntityLabels,
      },
    [],
  );

  const baseTheme = vertical?.theme ?? fallbackVertical.theme;
  const entityLabels = vertical?.entityLabels ?? fallbackVertical.entityLabels;

  const resolvedTheme = useMemo<ThittamTheme>(() => {
    if (!override) return baseTheme;
    return deepMerge(
      baseTheme as unknown as Record<string, unknown>,
      override as unknown as Record<string, unknown>,
    ) as unknown as ThittamTheme;
  }, [baseTheme, override]);

  // Apply CSS custom properties whenever the resolved theme changes
  useEffect(() => {
    const vars = themeToCssVars(resolvedTheme);
    const root = document.documentElement;

    for (const [prop, value] of Object.entries(vars)) {
      root.style.setProperty(prop, value);
    }

    // Cleanup: remove properties when the provider unmounts
    return () => {
      for (const prop of Object.keys(vars)) {
        root.style.removeProperty(prop);
      }
    };
  }, [resolvedTheme]);

  const setThemeOverride = useCallback(
    (next: Partial<ThittamTheme> | null) => setOverride(next),
    [],
  );

  const value = useMemo<ThemeContextValue>(
    () => ({ theme: resolvedTheme, entityLabels, setThemeOverride }),
    [resolvedTheme, entityLabels, setThemeOverride],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error("useTheme must be used within a <ThemeProvider>");
  }
  return ctx;
}
