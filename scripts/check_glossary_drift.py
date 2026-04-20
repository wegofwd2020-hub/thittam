#!/usr/bin/env python3
"""Scan docs for ALLCAPS acronyms that aren't defined in the glossary.

Reads `GLOSSARY.md` (markdown tables with `**Term**` in the first column),
then walks all `*.md` files under --docs and extracts capitalized
acronym-shaped tokens. Tokens present in docs but absent from the glossary
are reported as drift candidates.

Exit 0 = clean, 1 = drift detected. Report markdown goes to --out.

Ignore conventions:
  - Tokens in the `ALLOWLIST` below are never reported (HTTP verbs, file
    formats, currency codes, etc.) even when not in the glossary.
  - A line containing `glossary:ignore` immediately before a reference
    suppresses that single reference.
"""
from __future__ import annotations

import argparse
import re
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

# Match bolded first-column entries in any markdown table row. The glossary
# uses `**TERM**` or `**Term**` — pick them all up.
TERM_RE = re.compile(r"^\|\s*\*\*([^|*]+?)\*\*\s*\|", re.MULTILINE)

# Candidate acronyms: 3-8 chars, uppercase letter start. Negative lookbehind
# rejects tokens inside a structured ID like `FR-RPT-001` / `NFR-OBS-012`
# (where `RPT` and `OBS` are category codes, not project acronyms). Negative
# lookahead rejects tokens immediately followed by `-<digit>` (same reason).
ACRONYM_RE = re.compile(r"(?<![A-Z]-)\b[A-Z][A-Z0-9]{2,7}\b(?!-\d)")

IGNORE_TOKEN = "glossary:ignore"

# Words that look like acronyms but are not project-specific — HTTP verbs,
# file formats, currency codes, etc. Extend as false positives emerge.
ALLOWLIST = {
    # HTTP verbs + status-related
    "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS",
    "HTTP", "HTTPS", "REST", "GRPC",
    # File formats + data
    "JSON", "YAML", "TOML", "XML", "HTML", "CSS", "CSV", "PDF", "SQL",
    "MD", "PNG", "JPG", "JPEG", "SVG", "GIF",
    # Currency ISO codes (common ones — extend if you add more)
    "USD", "CAD", "INR", "GBP", "EUR", "AUD",
    # Country ISO codes (common ones)
    "US", "CA", "IN", "UK", "GB", "EU", "AU",
    # Time zones / dates
    "UTC", "EST", "EDT", "GMT", "PST", "PDT", "IST",
    # Common generic infra / ops
    "AWS", "GCP", "AZURE", "DNS", "TCP", "UDP", "TLS", "SSL",
    "IPV4", "IPV6", "IP", "ID", "URL", "URI",
    "CPU", "RAM", "GB", "MB", "KB", "TB",
    "OS", "VM", "K8S", "POD", "JVM", "NPM",
    # Misc generic
    "FAQ", "TBD", "TODO", "FIXME", "ETA", "ROI", "SLA", "SLO", "SLI",
    "ACID", "CRUD", "MVP", "NPS",
    # Log levels — generic across every project
    "INFO", "WARN", "WARNING", "ERROR", "DEBUG", "TRACE", "FATAL", "NOTICE",
    # SQL keywords + DML ops that commonly appear in prose discussion of schemas
    "NULL", "DEFAULT", "TRUE", "FALSE", "CHECK", "UNIQUE", "PRIMARY",
    "FOREIGN", "CASCADE", "ASC", "DESC", "NOW", "JSONB",
    "INSERT", "UPDATE", "SELECT", "MERGE", "TABLE", "INDEX",
    "CREATE", "ALTER", "DROP", "VIEW", "FROM", "WHERE",
    # Common English words that match the regex shape — source of noise
    "THE", "AND", "FOR", "NOT", "YES", "NO", "ONE", "TWO", "NEW", "OLD",
    "ALL", "ANY", "GET", "SET", "RUN", "USE", "HIGH", "LOW", "NEAR",
    "MID", "TOTAL", "NONE", "TRUE", "FALSE",
    # Standards-body prefixes — "ISO 3166", "ISO 8601" etc. appear verbatim
    "ISO", "ISO8601", "RFC", "ANSI",
}


def extract_glossary_terms(glossary_path: Path) -> set[str]:
    text = glossary_path.read_text()
    terms: set[str] = set()
    for m in TERM_RE.findall(text):
        # Normalise — glossary may list `DOP / DoP` or `bcrypt`. We want an
        # all-uppercase comparable key. Split on `/` and take any pure-caps
        # sub-word.
        for part in re.split(r"[/,\s]+", m.strip()):
            part = part.strip()
            if part and part.upper() == part and len(part) >= 2:
                terms.add(part)
    return terms


INLINE_CODE_RE = re.compile(r"`[^`\n]*`")
FENCE_RE = re.compile(r"^\s*```")


def scan_docs_for_acronyms(docs_root: Path) -> list[dict]:
    """Return {file, line, token} for each acronym occurrence in *prose*.

    Skips content inside fenced code blocks (```...```), strips inline `code`
    spans before matching, and honors `glossary:ignore` markers.
    """
    hits: list[dict] = []
    for f in sorted(docs_root.rglob("*.md")):
        try:
            lines = f.read_text().splitlines()
        except UnicodeDecodeError:
            continue
        in_fence = False
        for i, line in enumerate(lines):
            if FENCE_RE.match(line):
                in_fence = not in_fence
                continue
            if in_fence:
                continue
            prev = lines[i - 1] if i > 0 else ""
            if IGNORE_TOKEN in prev or IGNORE_TOKEN in line:
                continue
            stripped = INLINE_CODE_RE.sub(" ", line)
            for m in ACRONYM_RE.finditer(stripped):
                hits.append({"file": f, "line": i + 1, "token": m.group(0)})
    return hits


def render_report(
    undefined: dict[str, list[dict]],
    glossary_size: int,
    scan_size: int,
    docs_root: Path,
    project: str,
) -> str:
    now = datetime.now(timezone.utc).isoformat(timespec="seconds")
    lines = [
        f"# Glossary Drift — {project}",
        "",
        f"_Scan run: {now}_",
        "",
        f"Scanned {scan_size} acronym occurrences against {glossary_size} "
        "terms defined in the glossary.",
        "",
    ]
    if not undefined:
        lines += ["✅ **No drift detected.**", ""]
        return "\n".join(lines)

    lines += [
        f"## Acronyms mentioned in docs but not defined in the glossary "
        f"({len(undefined)})",
        "",
        "| Token | Mentions | First location |",
        "|---|---|---|",
    ]
    for token, refs in sorted(undefined.items(), key=lambda kv: (-len(kv[1]), kv[0])):
        first = refs[0]
        rel = first["file"].relative_to(docs_root)
        lines.append(f"| `{token}` | {len(refs)} | `{rel}:{first['line']}` |")
    lines.append("")
    lines += [
        "---",
        "",
        "**To silence a false positive** that isn't project-specific: add it "
        "to `ALLOWLIST` in `scripts/check_glossary_drift.py` (CI env / shell / "
        "infrastructure words live there already).",
        "",
        "**To silence one-off uses**: add `<!-- glossary:ignore -->` on the "
        "line immediately before the reference.",
        "",
        "**To fix real drift**: add the term + definition to the glossary, "
        "or rename/remove the undocumented acronym in the doc.",
        "",
    ]
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--glossary", required=True, help="Path to glossary markdown")
    ap.add_argument("--docs", required=True, help="Path to docs root")
    ap.add_argument("--out", required=True, help="Write report markdown here")
    ap.add_argument("--project", default="Project", help="Project name for report header")
    args = ap.parse_args()

    glossary = Path(args.glossary).resolve()
    docs = Path(args.docs).resolve()
    out = Path(args.out).resolve()

    defined = extract_glossary_terms(glossary) | ALLOWLIST
    hits = scan_docs_for_acronyms(docs)

    undefined: dict[str, list[dict]] = defaultdict(list)
    for h in hits:
        if h["token"] not in defined:
            undefined[h["token"]].append(h)

    report = render_report(undefined, len(defined), len(hits), docs, args.project)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(report)

    if undefined:
        print(
            f"Drift detected: {len(undefined)} undefined acronyms. Report: {out}",
            file=sys.stderr,
        )
        return 1
    print(f"Clean. Report: {out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
