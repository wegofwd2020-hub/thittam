"""Legal entity, owners, and the owner-equity accounts they imply.

A vertical's chart of accounts is entity-neutral: it stops at a ``3000``
equity parent. What sits under it depends on how the tenant's business is
owned and taxed, which is declared per tenant in the ``entity:`` block of
``company.yaml``::

    entity:
      tax_classification: partnership_1065
      members:
        a: {label: Member A, ownership_pct: 50}
        b: {label: Member B, ownership_pct: 50}

``members`` is a mapping (not a list) so that ``company.local.yaml`` can add a
member's real ``name`` key by key without restating the block; the committed
file keeps neutral labels because the repository is public.

Supported classifications
-------------------------
``disregarded_schedule_c``
    Exactly one owner. Generates ``3100 Owner's Contributions`` and
    ``3200 Owner's Draws``.
``partnership_1065``
    Two to nine members. For member *n* (1-based, in file order) generates
    ``30n0 Capital — <member>`` and ``31n0 Distributions — <member>``, plus
    ``3910 Current-Year Earnings (clearing)``, a ``6050 Guaranteed Payments
    to Partners`` parent (Form 1065 line 10) and ``605n Guaranteed Payments —
    <member>`` beneath it.

S-corporation and C-corporation books are not supported yet and are
rejected with a clear message rather than seeded with a wrong chart.

The block is optional: without it nothing is generated and the vertical's
own equity accounts are used unchanged, which keeps every other vertical's
tenants exactly as before. Verticals in :data:`ENTITY_REQUIRED_VERTICALS`
leave owner equity out of their chart, so their tenants must declare it.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Any

from .errors import Issue
from .vertical import AccountEntry

SCHEDULE_C = "disregarded_schedule_c"
PARTNERSHIP = "partnership_1065"
SUPPORTED = (SCHEDULE_C, PARTNERSHIP)
NOT_YET_SUPPORTED = ("s_corp_1120s", "c_corp_1120")

MAX_PARTNERS = 9  # member n owns codes 30n0 / 31n0 / 605n
EQUITY_PARENT = "3000"
EXPENSE_PARENT = "6000"
GUARANTEED_PAYMENTS_PARENT = "6050"
CLEARING_CODE = "3910"

# Verticals whose chart stops at the 3000 equity parent (see their YAML header).
ENTITY_REQUIRED_VERTICALS = ("software-development-us",)

_KEY_RE = re.compile(r"^[a-z][a-z0-9_]*$")
_HUNDRED = Decimal(100)


@dataclass(frozen=True)
class Member:
    """One owner of the business.

    Attributes:
        key: Stable identifier from ``company.yaml`` (e.g. ``"a"``).
        label: Public label used when no ``name`` is supplied.
        name: Real name, normally supplied only by ``company.local.yaml``.
        ownership_pct: Percentage interest, as an exact decimal.
    """

    key: str
    label: str
    name: str | None
    ownership_pct: Decimal

    @property
    def display(self) -> str:
        """Name used in account names: the real name if known, else the label."""
        return self.name or self.label


@dataclass(frozen=True)
class Entity:
    """How the business is owned and taxed.

    Attributes:
        tax_classification: One of :data:`SUPPORTED`.
        members: Owners in file order; percentages sum to exactly 100.
    """

    tax_classification: str
    members: tuple[Member, ...]

    @property
    def is_partnership(self) -> bool:
        return self.tax_classification == PARTNERSHIP


def parse_entity(raw: Any, issues: list[Issue]) -> Entity | None:
    """Validate the ``entity:`` block of a merged company mapping.

    Args:
        raw: The value of ``entity`` (``None`` when the block is absent).
        issues: Problems are appended here, located by dotted field path.

    Returns:
        The parsed :class:`Entity`, or ``None`` when the block is absent.
        When issues were appended the value is a placeholder: do not use it.
    """
    if raw is None:
        return None
    if not isinstance(raw, dict):
        issues.append(Issue("entity", "must be a mapping"))
        return None

    tc = raw.get("tax_classification")
    if tc in NOT_YET_SUPPORTED:
        issues.append(Issue("entity.tax_classification",
                            f"{tc!r} is not supported yet (supported: {', '.join(SUPPORTED)})"))
    elif tc not in SUPPORTED:
        issues.append(Issue("entity.tax_classification", f"must be one of {', '.join(SUPPORTED)}"))

    members = _parse_members(raw.get("members"), issues)
    if tc == SCHEDULE_C and members and len(members) != 1:
        issues.append(Issue("entity.members", f"{SCHEDULE_C} needs exactly one member, found {len(members)}"))
    if tc == PARTNERSHIP and members and not 2 <= len(members) <= MAX_PARTNERS:
        issues.append(Issue("entity.members",
                            f"{PARTNERSHIP} needs 2 to {MAX_PARTNERS} members, found {len(members)}"))
    if members:
        total = sum((m.ownership_pct for m in members), Decimal(0))
        if total != _HUNDRED:
            issues.append(Issue("entity.members", f"ownership_pct must total 100, got {total.normalize():f}"))

    return Entity(tax_classification=tc if tc in SUPPORTED else SCHEDULE_C, members=tuple(members))


def equity_accounts(entity: Entity | None) -> list[AccountEntry]:
    """Owner-equity (and, for a partnership, guaranteed-payment) accounts.

    Accounts are returned parents first; an absent entity yields none. Their
    parents ``3000`` and ``6000`` must exist in the vertical;
    :func:`check_entity_against_vertical` enforces that.
    """
    if entity is None:
        return []
    if not entity.is_partnership:
        return [
            AccountEntry("3100", "Owner's Contributions", "equity", EQUITY_PARENT),
            AccountEntry("3200", "Owner's Draws", "equity", EQUITY_PARENT),  # contra-equity
        ]
    out: list[AccountEntry] = []
    for n, m in enumerate(entity.members, start=1):
        out.append(AccountEntry(f"30{n}0", f"Capital — {m.display}", "equity", EQUITY_PARENT))
    for n, m in enumerate(entity.members, start=1):
        out.append(AccountEntry(f"31{n}0", f"Distributions — {m.display}", "equity", EQUITY_PARENT))
    out.append(AccountEntry(CLEARING_CODE, "Current-Year Earnings (clearing)", "equity", None))
    out.append(AccountEntry(GUARANTEED_PAYMENTS_PARENT, "Guaranteed Payments to Partners", "expense",
                            EXPENSE_PARENT))
    for n, m in enumerate(entity.members, start=1):
        out.append(AccountEntry(f"605{n}", f"Guaranteed Payments — {m.display}", "expense",
                                GUARANTEED_PAYMENTS_PARENT))
    return out


# Accounts the software-development-us vertical seeded before v1.1.0, when it
# assumed a single owner. A tenant seeded from that version keeps them (seed
# inserts never overwrite), so the ledger seed corrects them in place:
# renamed to the neutral v1.1.0 names, or — for a partnership, where they
# have no meaning — deactivated. History is kept: nothing is deleted.
LEGACY_RENAMES: tuple[tuple[str, str, str], ...] = (
    ("3000", "Owner's Capital", "Equity"),
    ("2300", "Due to Owner (reimbursable)", "Due to Owners (reimbursable)"),
)
LEGACY_SINGLE_OWNER: tuple[tuple[str, str], ...] = (
    ("3100", "Owner's Contributions"),
    ("3200", "Owner's Draws"),
)


def check_entity_against_vertical(entity: Entity | None, vertical_id: str,
                                  vertical_accounts: tuple[AccountEntry, ...]) -> list[Issue]:
    """Return problems combining the entity's accounts with the vertical's.

    Checks that a vertical needing an ``entity:`` block gets one, that every
    parent the entity accounts need exists with the right type, and that no
    generated code collides with a vertical account.
    """
    issues: list[Issue] = []
    if entity is None:
        if vertical_id in ENTITY_REQUIRED_VERTICALS:
            issues.append(Issue("entity", f"required for vertical {vertical_id}: its chart leaves owner "
                                          "equity to the tenant (see tools/tenant-onboard/README.md)"))
        return issues
    by_code = {a.code: a for a in vertical_accounts}
    generated = equity_accounts(entity)
    generated_codes = {a.code for a in generated}
    for acc in generated:
        if acc.code in by_code:
            issues.append(Issue("entity", f"generated account {acc.code} ({acc.name}) clashes with "
                                          f"vertical account {acc.code} ({by_code[acc.code].name})"))
        parent = acc.parent_code
        if parent is None or parent in generated_codes:
            continue
        if parent not in by_code:
            issues.append(Issue("entity", f"vertical has no account {parent}, needed as the parent of "
                                          f"{acc.code} ({acc.name})"))
        elif by_code[parent].account_type != acc.account_type:
            issues.append(Issue("entity", f"vertical account {parent} is {by_code[parent].account_type}, "
                                          f"but {acc.code} ({acc.name}) is {acc.account_type}"))
    return issues


# ── helpers ────────────────────────────────────────────────────────────────

def _parse_members(raw: Any, issues: list[Issue]) -> list[Member]:
    if not isinstance(raw, dict) or not raw:
        issues.append(Issue("entity.members", "required non-empty mapping of key -> member"))
        return []
    out: list[Member] = []
    for key, row in raw.items():
        where = f"entity.members.{key}"
        if not isinstance(key, str) or not _KEY_RE.match(key):
            issues.append(Issue(where, "key must be lowercase letters, digits and underscores"))
            continue
        if not isinstance(row, dict):
            issues.append(Issue(where, "must be a mapping"))
            continue
        before = len(issues)
        label = _text(row.get("label"), f"{where}.label", issues, required=True)
        name = _text(row.get("name"), f"{where}.name", issues, required=False)
        pct = _pct(row.get("ownership_pct"), f"{where}.ownership_pct", issues)
        if len(issues) == before:
            out.append(Member(key=key, label=label, name=name, ownership_pct=pct))
    return out


def _text(val: Any, field: str, issues: list[Issue], required: bool) -> str | None:
    if val is None:
        if required:
            issues.append(Issue(field, "required"))
        return None
    if not isinstance(val, str) or not val.strip():
        issues.append(Issue(field, "must be a non-empty string"))
        return None
    val = " ".join(val.split())
    if len(val) > 120:
        issues.append(Issue(field, "must be at most 120 characters (it becomes part of account names)"))
        return None
    return val


def _pct(val: Any, field: str, issues: list[Issue]) -> Decimal:
    """Parse a percentage exactly; floats go through ``str`` to avoid binary noise."""
    if isinstance(val, bool) or not isinstance(val, (int, float, str)):
        issues.append(Issue(field, "required number between 0 and 100"))
        return Decimal(0)
    try:
        pct = Decimal(str(val).strip())
    except InvalidOperation:
        issues.append(Issue(field, f"not a number: {val!r}"))
        return Decimal(0)
    if not pct.is_finite() or not Decimal(0) < pct <= _HUNDRED:
        issues.append(Issue(field, "must be greater than 0 and at most 100"))
        return Decimal(0)
    if pct.as_tuple().exponent < -4:
        issues.append(Issue(field, "at most 4 decimal places"))
        return Decimal(0)
    return pct
