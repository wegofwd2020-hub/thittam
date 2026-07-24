"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useSyncExternalStore,
} from "react";

interface AccessibilityState {
  dyslexiaMode: boolean;
  toggleDyslexia: () => void;
}

const AccessibilityContext = createContext<AccessibilityState>({
  dyslexiaMode: false,
  toggleDyslexia: () => {},
});

const STORAGE_KEY = "thittam-dyslexia-mode";

// The dyslexia preference lives in localStorage, not React state.
// useSyncExternalStore reads it without a mount-effect setState — that pattern
// triggers a second render before paint (a visible flash of un-styled content)
// and trips react-hooks/set-state-in-effect. getServerSnapshot returns the
// default so SSR and the first client render agree (no hydration mismatch).

const listeners = new Set<() => void>();

function emitChange() {
  for (const listener of listeners) listener();
}

function subscribe(callback: () => void) {
  listeners.add(callback);
  // Reflect changes made in other tabs, too.
  window.addEventListener("storage", callback);
  return () => {
    listeners.delete(callback);
    window.removeEventListener("storage", callback);
  };
}

function getSnapshot() {
  return localStorage.getItem(STORAGE_KEY) === "true";
}

function getServerSnapshot() {
  return false;
}

export function AccessibilityProvider({ children }: { children: React.ReactNode }) {
  const dyslexiaMode = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  // Apply/remove the document-level styles whenever the resolved preference
  // changes. This effect only mutates the DOM; it never calls setState, so
  // react-hooks/set-state-in-effect does not fire.
  useEffect(() => {
    applyDyslexiaStyles(dyslexiaMode);
  }, [dyslexiaMode]);

  const toggleDyslexia = useCallback(() => {
    const next = localStorage.getItem(STORAGE_KEY) !== "true";
    localStorage.setItem(STORAGE_KEY, String(next));
    emitChange();
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
