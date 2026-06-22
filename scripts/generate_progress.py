#!/usr/bin/env python3
"""Generate docs/PROGRESS.md for Thittam.

Features = GitHub issues (referenced in commit subjects as `#NN`).
Areas    = conventional-commit scopes (iam, billing, web, infra, ...).

A feature (issue) appearing in 2+ commits is counted as iterated/redesigned.
`gh` CLI must be authenticated against github.com/wegofwd2020-hub/thittam.
"""
from __future__ import annotations

import csv
import json
import re
import subprocess
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

# Optional add-on: an "in flight" section sourced from the orchestrator's ledger
# in .orchestration/. Guarded so a missing add-on can never break the nightly
# build; build_section itself never raises.
try:
    from progress_inflight import build_section
except Exception:  # pragma: no cover - add-on is optional
    build_section = None

REPO = Path(__file__).resolve().parents[1]
OUTPUT = REPO / "docs" / "PROGRESS.md"
OUTPUT_SCOPES_CSV = REPO / "docs" / "PROGRESS_scopes.csv"
OUTPUT_ISSUES_CSV = REPO / "docs" / "PROGRESS_issues.csv"
GH_REPO = "wegofwd2020-hub/thittam"

ISSUE_RE = re.compile(r"#(\d+)")
SCOPE_RE = re.compile(r"^[a-z]+\(([a-z0-9\-]+)\)")


def fetch_issues() -> dict[int, dict]:
    raw = subprocess.check_output(
        [
            "gh", "issue", "list",
            "--repo", GH_REPO,
            "--state", "all",
            "--limit", "1000",
            "--json", "number,title,state,createdAt,closedAt,labels",
        ],
        text=True,
    )
    prs_raw = subprocess.check_output(
        [
            "gh", "pr", "list",
            "--repo", GH_REPO,
            "--state", "all",
            "--limit", "1000",
            "--json", "number,title,state,createdAt,closedAt,labels",
        ],
        text=True,
    )
    out: dict[int, dict] = {}
    for item in json.loads(raw) + json.loads(prs_raw):
        out[item["number"]] = item
    return out


def get_commits() -> list[dict]:
    raw = subprocess.check_output(
        ["git", "log", "--pretty=format:%H|%aI|%s"],
        cwd=REPO, text=True,
    )
    commits = []
    for line in raw.splitlines():
        h, d, s = line.split("|", 2)
        scope_m = SCOPE_RE.match(s)
        commits.append({
            "hash": h,
            "date": datetime.fromisoformat(d).date().isoformat(),
            "subject": s,
            "scope": scope_m.group(1) if scope_m else None,
            "issues": [int(n) for n in ISSUE_RE.findall(s)],
        })
    return commits


def render(issues: dict[int, dict], commits: list[dict]) -> str:
    per_issue: dict[int, list[dict]] = defaultdict(list)
    per_scope: dict[str, list[dict]] = defaultdict(list)
    issue_scopes: dict[int, set[str]] = defaultdict(set)
    for c in commits:
        for n in c["issues"]:
            per_issue[n].append(c)
            if c["scope"]:
                issue_scopes[n].add(c["scope"])
        if c["scope"]:
            per_scope[c["scope"]].append(c)

    now = datetime.now(timezone.utc).isoformat(timespec="seconds")
    lines: list[str] = []
    lines.append("# Thittam — Progress Chart")
    lines.append("")
    lines.append(f"_Auto-generated {now}. Regenerated nightly at 21:00 EST._")
    lines.append("")
    lines.append(
        "Features are GitHub issues/PRs from "
        f"[`{GH_REPO}`](https://github.com/{GH_REPO}). Areas are conventional-commit scopes. "
        "Script: `scripts/generate_progress.py`."
    )
    lines.append("")

    lines.append("## Activity by area (service / scope)")
    lines.append("")
    lines.append("```mermaid")
    lines.append("gantt")
    lines.append("    title Thittam — Activity Windows by Scope")
    lines.append("    dateFormat YYYY-MM-DD")
    lines.append("    axisFormat %Y-%m")
    for scope in sorted(per_scope):
        cs = per_scope[scope]
        ds = sorted(c["date"] for c in cs)
        first, last = ds[0], ds[-1]
        if first == last:
            last_dt = datetime.fromisoformat(first).date()
            last = last_dt.replace(day=min(last_dt.day + 1, 28)).isoformat()
        safe_scope = scope.replace("-", "_")
        lines.append(f"    {scope:<18} :active, s_{safe_scope}, {first}, {last}")
    lines.append("```")
    lines.append("")

    lines.append("## Scope summary")
    lines.append("")
    lines.append("| Scope | Commits | First | Last | Issues touched |")
    lines.append("|---|---|---|---|---|")
    for scope in sorted(per_scope, key=lambda k: -len(per_scope[k])):
        cs = per_scope[scope]
        ds = sorted(c["date"] for c in cs)
        issues_here = {n for c in cs for n in c["issues"]}
        lines.append(
            f"| `{scope}` | {len(cs)} | {ds[0]} | {ds[-1]} | {len(issues_here)} |"
        )
    lines.append("")

    lines.append("## Feature summary (by issue)")
    lines.append("")
    lines.append("| Issue | Title | State | Commits | First | Last | Scopes |")
    lines.append("|---|---|---|---|---|---|---|")
    feature_rows = []
    for num, cs in per_issue.items():
        meta = issues.get(num, {})
        title = meta.get("title", "(not found on GitHub)")
        state = meta.get("state", "—")
        ds = sorted(c["date"] for c in cs)
        scopes_str = ", ".join(sorted(issue_scopes[num])) or "—"
        feature_rows.append((num, title, state, len(cs), ds[0], ds[-1], scopes_str))
    feature_rows.sort(key=lambda r: (-r[3], -r[0]))
    for num, title, state, cnt, first, last, scopes_str in feature_rows:
        title_short = title.replace("|", "\\|")[:70]
        url = f"https://github.com/{GH_REPO}/issues/{num}"
        lines.append(
            f"| [#{num}]({url}) | {title_short} | {state} | {cnt} | {first} | {last} | {scopes_str} |"
        )
    lines.append("")

    lines.append("## Redesign leaderboard")
    lines.append("")
    lines.append("Issues/PRs referenced in 2+ commits — each extra commit is an iteration or rework.")
    lines.append("")
    lines.append("| Issue | Title | Commits | First | Last | Span (days) |")
    lines.append("|---|---|---|---|---|---|")
    rows = [r for r in feature_rows if r[3] >= 2]
    if not rows:
        lines.append("| — | — | — | — | — | — |")
    for num, title, state, cnt, first, last, _ in rows:
        span = (
            datetime.fromisoformat(last).date()
            - datetime.fromisoformat(first).date()
        ).days
        url = f"https://github.com/{GH_REPO}/issues/{num}"
        lines.append(
            f"| [#{num}]({url}) | {title[:60].replace('|', '\\|')} | {cnt} | {first} | {last} | {span} |"
        )
    lines.append("")

    lines.append("## Per-scope detail")
    lines.append("")
    for scope in sorted(per_scope):
        cs = per_scope[scope]
        lines.append(f"### `{scope}`")
        lines.append("")
        lines.append(f"- **Commits:** {len(cs)}")
        issues_here = sorted({n for c in cs for n in c["issues"]})
        if issues_here:
            titles = []
            for n in issues_here:
                t = issues.get(n, {}).get("title", "(unknown)")
                titles.append(f"  - [#{n}](https://github.com/{GH_REPO}/issues/{n}) — {t[:80]}")
            lines.append(f"- **Issues referenced ({len(issues_here)}):**")
            lines.extend(titles)
        lines.append("")

    return "\n".join(lines) + "\n"


def write_csvs(issues: dict[int, dict], commits: list[dict]) -> None:
    """Write PROGRESS_scopes.csv + PROGRESS_issues.csv for spreadsheet analysis.

    Same data as the markdown tables, flattened to CSV so it can be pulled
    into Google Sheets via `=IMPORTDATA(...)` or imported manually.
    """
    per_issue: dict[int, list[dict]] = defaultdict(list)
    per_scope: dict[str, list[dict]] = defaultdict(list)
    issue_scopes: dict[int, set[str]] = defaultdict(set)
    for c in commits:
        for n in c["issues"]:
            per_issue[n].append(c)
            if c["scope"]:
                issue_scopes[n].add(c["scope"])
        if c["scope"]:
            per_scope[c["scope"]].append(c)

    # Scopes sheet — one row per scope.
    with OUTPUT_SCOPES_CSV.open("w", newline="") as f:
        w = csv.writer(f)
        w.writerow([
            "scope", "commit_count", "first", "last", "issue_count",
        ])
        for scope in sorted(per_scope, key=lambda k: -len(per_scope[k])):
            cs = per_scope[scope]
            ds = sorted(c["date"] for c in cs)
            issues_here = {n for c in cs for n in c["issues"]}
            w.writerow([scope, len(cs), ds[0], ds[-1], len(issues_here)])

    # Issues sheet — one row per GitHub issue/PR referenced in commits.
    with OUTPUT_ISSUES_CSV.open("w", newline="") as f:
        w = csv.writer(f)
        w.writerow([
            "issue_number", "title", "state",
            "commit_count", "first", "last", "span_days", "scopes",
        ])
        for num, cs in sorted(per_issue.items(), key=lambda kv: -len(kv[1])):
            meta = issues.get(num, {})
            title = meta.get("title", "(not found on GitHub)")
            state = meta.get("state", "")
            ds = sorted(c["date"] for c in cs)
            span = (
                datetime.fromisoformat(ds[-1]).date()
                - datetime.fromisoformat(ds[0]).date()
            ).days
            scopes_str = "; ".join(sorted(issue_scopes[num]))
            w.writerow([num, title, state, len(cs), ds[0], ds[-1], span, scopes_str])


def main() -> None:
    issues = fetch_issues()
    commits = get_commits()
    document = render(issues, commits)
    if build_section is not None:
        document += "\n" + build_section(REPO / ".orchestration")
    OUTPUT.write_text(document)
    write_csvs(issues, commits)
    print(
        f"Wrote {OUTPUT} + {OUTPUT_SCOPES_CSV.name} + {OUTPUT_ISSUES_CSV.name} "
        f"— {len(issues)} issues/PRs, {len(commits)} commits scanned."
    )


if __name__ == "__main__":
    main()
