"use client";

import { useSyncExternalStore } from "react";

// Product name, version, and the local date/time, scrolling along the bottom of the sidebar.
// The clock is client-only: it renders a placeholder on the server and first paint, so
// the server and browser markup match.
// Ticks once a second. Server snapshot is null, so the server and first client render match.
function subscribeToClock(onTick: () => void) {
  const id = setInterval(onTick, 1000);
  return () => clearInterval(id);
}

export function SidebarTicker() {
  const seconds = useSyncExternalStore(
    subscribeToClock,
    () => Math.floor(Date.now() / 1000),
    () => null,
  );

  const version = process.env.NEXT_PUBLIC_APP_VERSION ?? "dev";
  const clock = seconds !== null
    ? new Date(seconds * 1000).toLocaleString(undefined, {
        weekday: "short",
        day: "numeric",
        month: "short",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      })
    : "—";
  const text = `Thittam ${version}  ·  ${clock}`;

  return (
    <div
      className="sidebar-ticker mt-2 overflow-hidden border-t pt-2 text-[11px]"
      style={{
        borderColor: "var(--thittam-sidebar-active-bg, rgba(255,255,255,0.08))",
        color: "var(--thittam-sidebar-text, #94a3b8)",
      }}
      aria-label={text}
    >
      <div className="sidebar-ticker-track whitespace-nowrap">
        <span className="px-4">{text}</span>
        <span className="px-4" aria-hidden="true">{text}</span>
      </div>
    </div>
  );
}
