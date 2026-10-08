"""Vertical YAML → onboarding model and ``vertical_definitions`` migration.

The Go validator in ``pkg/vertical`` is the authority on whether a vertical is
well-formed (``make validate-verticals``). This module re-checks only what the
onboarding generator itself depends on — the chart of accounts and the stage
ids — so the generator fails with a clear message instead of emitting SQL that
breaks halfway through a ``psql`` run.

It also renders the shared migration that registers a vertical in the
``vertical_definitions`` table. The JSONB ``config`` column holds everything
under the ``vertical:`` key *except* ``id``, ``name``, ``version`` and
``description`` (the convention set by ``migrations/shared/003``).
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from . import sql
from .errors import Issue, VerticalConfigError
from .yamlio import load_yaml_mapping

ACCOUNT_TYPES = ("asset", "liability", "equity", "revenue", "expense")
_ID_RE = re.compile(r"^[a-z][a-z0-9-]*$")
_CODE_RE = re.compile(r"^[0-9A-Za-z][0-9A-Za-z.-]*$")
_TOP_LEVEL_META = ("id", "name", "version", "description")


@dataclass(frozen=True)
class AccountEntry:
    """One chart-of-accounts row from ``default_chart_of_accounts``."""

    code: str
    name: str
    account_type: str
    parent_code: str | None = None


@dataclass(frozen=True)
class Vertical:
    """The parts of a vertical definition the generator needs.

    Attributes:
        id: Vertical id, e.g. ``software-development-us``.
        name: Display name.
        version: Semantic version string.
        description: Free-text description (may be empty).
        config: JSON-ready mapping stored in ``vertical_definitions.config``.
        accounts: Chart of accounts in seeding order (parents first).
        stage_ids: Ids of ``phase_types`` in declaration order.
        source: Path the vertical was loaded from (for messages).
    """

    id: str
    name: str
    version: str
    description: str
    config: dict[str, Any]
    accounts: tuple[AccountEntry, ...]
    stage_ids: tuple[str, ...]
    source: str = field(default="<memory>")


def load_vertical(path: str | Path) -> Vertical:
    """Load and check a vertical YAML file.

    Args:
        path: Path to a ``pkg/vertical/configs/*.yaml`` file.

    Returns:
        The parsed :class:`Vertical`.

    Raises:
        InputFileError: The file cannot be read or parsed.
        VerticalConfigError: The vertical fails a rule the generator needs.
    """
    data = load_yaml_mapping(path)
    return parse_vertical(data, source=str(path))


def parse_vertical(data: dict[str, Any], source: str = "<memory>") -> Vertical:
    """Validate an already-parsed vertical document.

    Args:
        data: Mapping with a top-level ``vertical`` key.
        source: Label used in error messages.

    Returns:
        The parsed :class:`Vertical`.

    Raises:
        VerticalConfigError: With every issue found.
    """
    issues: list[Issue] = []
    v = data.get("vertical")
    if not isinstance(v, dict):
        raise VerticalConfigError(source, [Issue("vertical", "missing or not a mapping")])

    meta: dict[str, str] = {}
    for key in ("id", "name", "version"):
        val = v.get(key)
        if not isinstance(val, str) or not val.strip():
            issues.append(Issue(f"vertical.{key}", "required non-empty string"))
        else:
            meta[key] = val.strip()
    if "id" in meta and not _ID_RE.match(meta["id"]):
        issues.append(Issue("vertical.id", "must be lowercase letters, digits and hyphens"))
    description = v.get("description") or ""
    if not isinstance(description, str):
        issues.append(Issue("vertical.description", "must be a string"))
        description = ""

    accounts = _parse_accounts(v.get("default_chart_of_accounts"), issues)
    stage_ids = _parse_stage_ids(v.get("phase_types"), issues)

    if issues:
        raise VerticalConfigError(source, issues)

    config = {k: val for k, val in v.items() if k not in _TOP_LEVEL_META}
    return Vertical(
        id=meta["id"],
        name=meta["name"],
        version=meta["version"],
        description=" ".join(description.split()),
        config=config,
        accounts=tuple(accounts),
        stage_ids=tuple(stage_ids),
        source=source,
    )


def _parse_accounts(raw: Any, issues: list[Issue]) -> list[AccountEntry]:
    """Validate the chart of accounts; appends to ``issues`` and returns rows."""
    base = "vertical.default_chart_of_accounts"
    if not isinstance(raw, list) or not raw:
        issues.append(Issue(base, "required non-empty list"))
        return []

    out: list[AccountEntry] = []
    seen: dict[str, AccountEntry] = {}
    for i, row in enumerate(raw):
        where = f"{base}[{i}]"
        if not isinstance(row, dict):
            issues.append(Issue(where, "must be a mapping"))
            continue
        code, name, atype, parent = (row.get(k) for k in ("code", "name", "account_type", "parent_code"))
        row_ok = True
        if not isinstance(code, str) or not _CODE_RE.match(code):
            issues.append(Issue(f"{where}.code", f"invalid account code {code!r} (quote numeric codes in YAML)"))
            row_ok = False
        elif code in seen:
            issues.append(Issue(f"{where}.code", f"duplicate account code {code!r}"))
            row_ok = False
        if not isinstance(name, str) or not name.strip():
            issues.append(Issue(f"{where}.name", "required non-empty string"))
            row_ok = False
        if atype not in ACCOUNT_TYPES:
            issues.append(Issue(f"{where}.account_type", f"must be one of {', '.join(ACCOUNT_TYPES)}"))
            row_ok = False
        if parent is not None:
            if not isinstance(parent, str):
                issues.append(Issue(f"{where}.parent_code", "must be a quoted string or null"))
                row_ok = False
            elif parent not in seen:
                issues.append(Issue(
                    f"{where}.parent_code",
                    f"parent {parent!r} must be defined earlier in the list "
                    "(the ledger seeder resolves parents in order)",
                ))
                row_ok = False
            elif atype in ACCOUNT_TYPES and seen[parent].account_type != atype:
                issues.append(Issue(
                    f"{where}.parent_code",
                    f"parent {parent!r} is {seen[parent].account_type}, child is {atype}",
                ))
                row_ok = False
        if row_ok:
            entry = AccountEntry(code=code, name=name.strip(), account_type=atype, parent_code=parent)
            seen[code] = entry
            out.append(entry)
    return out


def _parse_stage_ids(raw: Any, issues: list[Issue]) -> list[str]:
    """Collect ``phase_types[*].id``; appends to ``issues`` on problems."""
    if not isinstance(raw, list) or not raw:
        issues.append(Issue("vertical.phase_types", "required non-empty list"))
        return []
    ids: list[str] = []
    for i, row in enumerate(raw):
        pid = row.get("id") if isinstance(row, dict) else None
        if not isinstance(pid, str) or not pid:
            issues.append(Issue(f"vertical.phase_types[{i}].id", "required string"))
        else:
            ids.append(pid)
    return ids


def render_migration(vertical: Vertical, source_rel: str) -> tuple[str, str]:
    """Render the up/down shared migration that registers ``vertical``.

    The up migration is an idempotent upsert, so re-running it after editing
    the YAML (and regenerating) updates the stored config in place.

    Args:
        vertical: The parsed vertical.
        source_rel: Repo-relative path of the YAML, quoted in the header.

    Returns:
        ``(up_sql, down_sql)``.

    Raises:
        RenderError: If any value cannot be rendered safely.
    """
    header = (
        f"-- GENERATED by tools/tenant-onboard from {source_rel}\n"
        "-- Do not edit by hand: change the YAML, then run `make vertical-migrations`.\n"
        "-- `make check-vertical-migrations` fails CI if this file drifts from the YAML.\n"
    )
    up = (
        header
        + f"--\n-- Registers the {vertical.id} vertical in vertical_definitions.\n\n"
        "INSERT INTO vertical_definitions (id, name, version, description, config, is_active)\n"
        "VALUES (\n"
        f"    {sql.text(vertical.id)},\n"
        f"    {sql.text(vertical.name)},\n"
        f"    {sql.text(vertical.version)},\n"
        f"    {sql.text(vertical.description or None)},\n"
        f"    {sql.jsonb(vertical.config, indent=2)},\n"
        "    true\n"
        ")\n"
        "ON CONFLICT (id) DO UPDATE\n"
        "   SET name        = EXCLUDED.name,\n"
        "       version     = EXCLUDED.version,\n"
        "       description = EXCLUDED.description,\n"
        "       config      = EXCLUDED.config,\n"
        "       is_active   = true;\n"
    )
    down = (
        header
        + "--\n-- Fails (FK on tenant_verticals) while any tenant is still bound to this\n"
        "-- vertical. That is intentional: rebind or remove those tenants first.\n\n"
        f"DELETE FROM vertical_definitions WHERE id = {sql.text(vertical.id)};\n"
    )
    return up, down
