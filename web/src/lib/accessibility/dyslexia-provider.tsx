"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";

interface AccessibilityState {
  dyslexiaMode: boolean;
  toggleDyslexia: () => void;
}

const AccessibilityContext = createContext<AccessibilityState>({
  dyslexiaMode: false,
  toggleDyslexia: () => {},
});

const STORAGE_KEY = "thittam-dyslexia-mode";

export function AccessibilityProvider({ children }: { children: React.ReactNode }) {
  const [dyslexiaMode, setDyslexiaMode] = useState(false);

  // Load preference from localStorage on mount
  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "true") {
      setDyslexiaMode(true);
      applyDyslexiaStyles(true);
    }
  }, []);

  const toggleDyslexia = useCallback(() => {
    setDyslexiaMode((prev) => {
      const next = !prev;
      localStorage.setItem(STORAGE_KEY, String(next));
      applyDyslexiaStyles(next);
      return next;
    });
  }, []);

  return (
    <AccessibilityContext.Provider value={{ dyslexiaMode, toggleDyslexia }}>
      {children}
    </AccessibilityContext.Provider>
  );
}

export function useAccessibility() {
  return useContext(AccessibilityContext);
}

// Applies/removes dyslexia-friendly styles on the document root.
function applyDyslexiaStyles(enabled: boolean) {
  const root = document.documentElement;

  if (enabled) {
    root.classList.add("dyslexia-mode");
    root.style.setProperty("--font-heading", "'OpenDyslexic', sans-serif");
    root.style.setProperty("--font-body", "'OpenDyslexic', serif");
    root.style.setProperty("--font-mono", "'OpenDyslexic Mono', monospace");
    root.style.setProperty("--letter-spacing", "0.05em");
    root.style.setProperty("--line-height", "1.8");
  } else {
    root.classList.remove("dyslexia-mode");
    root.style.removeProperty("--font-heading");
    root.style.removeProperty("--font-body");
    root.style.removeProperty("--font-mono");
    root.style.removeProperty("--letter-spacing");
    root.style.removeProperty("--line-height");
  }
}
