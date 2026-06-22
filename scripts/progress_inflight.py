#!/usr/bin/env python3
"""Render an "agent orchestration — in flight" section for docs/PROGRESS.md.

This is a self-contained, standard-library-only helper meant to be copied into
the consuming project's ``scripts/`` directory, next to ``generate_progress.py``.
It reads the orchestrator's state - the ``manifest.json`` taskforge wrote and the
``.orchestration.json`` ledger the orchestrator maintains - and renders a
Markdown section describing the agent work that is *currently in flight*
(queued, in progress, in rework, or escalated).

It intentionally has no dependency on the orchestrator package: the two JSON
files are the contract between the tools, mirroring how the rest of the toolkit
is decoupled.

Why this complements PROGRESS.md: the existing generator reports on *merged
history* (git log) and infers rework from "a ticket ID reappears in 2+
commits". This section reports the orthogonal, forward-looking half - work that
has not merged yet - and records rework *with its cause* (the gate verdict),
rather than inferring it after the fact.

Design rule: this helper must NEVER raise. A reporting add-on must not be able
to fail the nightly build, so every error path degrades to a safe placeholder
section.

Usage (standalone):
    python3 progress_inflight.py --tasks-dir .orchestration
    python3 progress_inflight.py --tasks-dir .orchestration --out section.md

Usage (imported by generate_progress.py):
    from progress_inflight import build_section
    document += build_section(Path(".orchestration"))
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

_LEDGER_NAME = ".orchestration.json"
_MANIFEST_NAME = "manifest.json"
_HEADING = "## Agent orchestration \u2014 in flight"

# Statuses that count as "open" agent work worth surfacing, most-attention-first.
_INFLIGHT_ORDER: dict[str, int] = {
    "escalated": 0,
    "changes-requested": 1,
    "in-progress": 2,
    "pending": 3,
}
# Statuses that are done/parked - counted in the summary but not the table.
_TERMINAL = {"approved", "skipped"}

_SEVERITY_ORDER: dict[str, int] = {"critical": 0, "high": 1, "medium": 2, "low": 3}


@dataclass(frozen=True)
class _Row:
    """One merged task row (manifest title/severity + ledger status)."""

    number: int
    title: str
    severity: str
    status: str
    last_gate: str


def _read_json(path: Path) -> Any:
    """Read and parse a JSON file, returning ``None`` on any failure.

    Args:
        path: The file to read.

    Returns:
        The parsed JSON, or ``None`` if the file is missing or invalid.
    """
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None


def _merge_rows(manifest: Any, ledger: Any) -> list[_Row]:
    """Merge manifest task metadata with ledger status into rows.

    Args:
        manifest: Parsed ``manifest.json`` (expects a ``tasks`` array), or None.
        ledger: Parsed ``.orchestration.json`` (expects a ``tasks`` map), or None.

    Returns:
        One :class:`_Row` per manifest task, with status taken from the ledger
        (defaulting to ``"pending"`` when the ledger has no entry).
    """
    tasks = manifest.get("tasks") if isinstance(manifest, dict) else None
    if not isinstance(tasks, list):
        return []
    ledger_tasks = ledger.get("tasks", {}) if isinstance(ledger, dict) else {}

    rows: list[_Row] = []
    for entry in tasks:
        if not isinstance(entry, dict) or "number" not in entry:
            continue
        try:
            number = int(entry["number"])
        except (TypeError, ValueError):
            continue
        state = ledger_tasks.get(str(number), {}) if isinstance(ledger_tasks, dict) else {}
        rows.append(_Row(
            number=number,
            title=str(entry.get("title", "")).strip() or "(untitled)",
            severity=str(entry.get("severity", "")).strip() or "?",
            status=str(state.get("status", "pending")).strip().lower() or "pending",
            last_gate=str(state.get("last_gate", "")).strip(),
        ))
    return rows


def _summary_line(rows: list[_Row], now: datetime) -> str:
    """Build the one-line summary of counts by status."""
    counts: dict[str, int] = {}
    for row in rows:
        counts[row.status] = counts.get(row.status, 0) + 1
    open_count = sum(c for s, c in counts.items() if s not in _TERMINAL)
    approved = counts.get("approved", 0)
    parts = [f"{c} {s}" for s, c in sorted(counts.items()) if s not in _TERMINAL and c]
    detail = f" ({', '.join(parts)})" if parts else ""
    stamp = now.strftime("%Y-%m-%d %H:%M UTC")
    return (f"_As of {stamp}: {open_count} agent task(s) in flight{detail}; "
            f"{approved} approved._")


def _table(rows: list[_Row]) -> list[str]:
    """Build the Markdown table of in-flight tasks (excludes terminal ones)."""
    inflight = [r for r in rows if r.status not in _TERMINAL]
    if not inflight:
        return ["_No agent tasks in flight._"]
    inflight.sort(key=lambda r: (
        _INFLIGHT_ORDER.get(r.status, 99),
        _SEVERITY_ORDER.get(r.severity, 99),
        r.number,
    ))
    lines = ["| # | status | sev | last gate | title |",
             "| --- | --- | --- | --- | --- |"]
    for r in inflight:
        gate = r.last_gate or "\u2014"
        title = r.title.replace("|", "\\|")
        lines.append(f"| {r.number} | {r.status} | {r.severity} | {gate} | {title} |")
    return lines


def _escalation_callout(rows: list[_Row]) -> list[str]:
    """Build a callout listing escalated tasks, if any."""
    escalated = [r.number for r in rows if r.status == "escalated"]
    if not escalated:
        return []
    nums = ", ".join(f"#{n}" for n in sorted(escalated))
    return ["", f"> \u26a0\ufe0f {len(escalated)} task(s) escalated to human "
                f"review: {nums}."]


def build_section(tasks_dir: str | Path, now: datetime | None = None) -> str:
    """Render the in-flight Markdown section for a tasks directory.

    Never raises: any error degrades to a safe placeholder section so the
    nightly progress build cannot fail because of this add-on.

    Args:
        tasks_dir: Directory holding ``manifest.json`` and ``.orchestration.json``.
        now: Timestamp to stamp the summary with. Defaults to the current UTC time.

    Returns:
        A Markdown section beginning with the standard heading.
    """
    when = now or datetime.now(timezone.utc)
    try:
        directory = Path(tasks_dir)
        manifest = _read_json(directory / _MANIFEST_NAME)
        ledger = _read_json(directory / _LEDGER_NAME)

        if manifest is None:
            return (f"{_HEADING}\n\n_No agent orchestration data found at "
                    f"`{directory}`._\n")

        rows = _merge_rows(manifest, ledger)
        if not rows:
            return f"{_HEADING}\n\n_No agent tasks recorded._\n"

        body = [_HEADING, "", _summary_line(rows, when), ""]
        body += _table(rows)
        body += _escalation_callout(rows)
        return "\n".join(body).rstrip() + "\n"
    except (OSError, ValueError, TypeError, KeyError) as exc:
        # Degrade safely; surface the cause as a comment without breaking render.
        return f"{_HEADING}\n\n<!-- in-flight section unavailable: {exc} -->\n"


def main(argv: list[str] | None = None) -> int:
    """CLI entry point: print (or write) the in-flight section.

    Args:
        argv: Argument vector excluding the program name.

    Returns:
        Always ``0`` - this helper does not fail.
    """
    parser = argparse.ArgumentParser(
        description="Render the agent-orchestration in-flight section for PROGRESS.md.",
    )
    parser.add_argument("--tasks-dir", default=".orchestration",
                        help="Directory with manifest.json and .orchestration.json "
                             "(default: .orchestration).")
    parser.add_argument("--out", default=None,
                        help="Write to this file instead of stdout.")
    args = parser.parse_args(sys.argv[1:] if argv is None else argv)

    section = build_section(args.tasks_dir)
    if args.out:
        try:
            Path(args.out).write_text(section, encoding="utf-8")
        except OSError as exc:
            print(f"warning: could not write {args.out}: {exc}", file=sys.stderr)
            print(section)
    else:
        print(section)
    return 0


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
