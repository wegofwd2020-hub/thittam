"""Start-up expense loader: expense extract CSV -> reimbursable-liability drafts.

Each loadable row becomes one *draft* journal entry: Dr ``1310`` Start-up &
Organizational Costs (company share) / Cr ``2300`` Due to Owners (reimbursable),
with the credit tagged to the owed member's party record. These are liabilities
owed under Operating Agreement §4.5, never capital contributions.

Gates (any failure blocks writing; nothing is partially written):

* a ``TBD`` attribution on any loadable row (the owner decides each one);
* a reconciled total that differs from the expected total (never forced);
* a company share that rounds to zero or is negative.

Rows are rejected (logged with their line number) when date, amount or status
is missing or malformed. Only ``candidate`` and ``candidate-verify`` rows with a
non-zero attribution load; ``excluded-*`` and ``pending*`` rows never do, and
Michigan Secretary of State rows are held regardless of status.

Amounts are :class:`decimal.Decimal` throughout. Original amount and company
share are both kept on every row (narration and manifest).
"""

from __future__ import annotations

import csv
import io
import re
import uuid
from dataclasses import dataclass
from datetime import date
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation
from pathlib import Path
from typing import Iterable

from . import sql
from .company import Company
from .entity import Member
from .errors import InputFileError, RenderError

HEADER = (
    "date", "vendor", "reference", "description", "amount_usd",
    "status", "attribution", "source", "notes",
)
LOADABLE_STATUSES = frozenset({"candidate", "candidate-verify"})
SKIP_STATUS_PREFIXES = ("excluded-", "pending")
# Held until the owner confirms them (owner rule: not loaded under any status).
HELD_VENDOR_PREFIXES = ("Michigan Secretary of State",)
# Chart codes from software-development-us (see pkg/vertical/configs).
DEBIT_ACCOUNT = "1310"    # Start-up & Organizational Costs (§195 / §709)
PAYABLE_ACCOUNT = "2300"  # Due to Owners (reimbursable)
ENTRY_PREFIX = "SU-"
# Total of the v2 candidate extract. Reconciliation is against this, not forced.
V2_CANDIDATE_TOTAL = Decimal("1752.57")
CENT = Decimal("0.01")
_PCT_RE = re.compile(r"^(\d{1,3}(?:\.\d{1,4})?)%$")


class _RowError(Exception):
    """Internal: one row failed a per-row check. Converted to a Rejection."""


@dataclass(frozen=True)
class Row:
    """One parsed extract row. ``line`` is the 1-based file line (header = 1)."""

    line: int
    when: date
    vendor: str
    reference: str
    description: str
    amount: Decimal
    status: str
    attribution: str
    notes: str


@dataclass(frozen=True)
class Rejection:
    line: int
    reason: str


@dataclass(frozen=True)
class Skip:
    line: int
    vendor: str
    reference: str
    amount: Decimal
    reason: str


@dataclass(frozen=True)
class Loadable:
    """A row that passes every gate. ``fraction`` is in (0, 1]."""

    row: Row
    fraction: Decimal
    company_amount: Decimal


@dataclass(frozen=True)
class Plan:
    loadable: tuple[Loadable, ...]
    skipped: tuple[Skip, ...]
    tbd: tuple[Row, ...]
    rejections: tuple[Rejection, ...]


@dataclass(frozen=True)
class Reconciliation:
    """Candidate total (loadable + TBD rows, original amounts) vs ``expected``.

    TBD rows count here because they are candidates awaiting an attribution
    decision; leaving them out would make the total look short for the wrong reason.
    """

    expected: Decimal
    candidate_total: Decimal
    loadable_total: Decimal
    company_total: Decimal

    @property
    def difference(self) -> Decimal:
        return self.candidate_total - self.expected

    @property
    def matches(self) -> bool:
        return self.difference == 0


@dataclass(frozen=True)
class Entry:
    line: int
    when: date
    vendor: str
    reference: str
    amount: Decimal
    fraction: Decimal
    company_amount: Decimal
    entry_number: str
    narration: str


# ── reading ───────────────────────────────────────────────────────────────

def read_extract(path: str | Path) -> tuple[list[Row], list[Rejection]]:
    """Read and schema-check the extract. Bad rows are returned, not raised.

    Raises:
        InputFileError: The file is missing, unreadable, not UTF-8, not valid
            CSV, or lacks a required column.
    """
    p = Path(path)
    rows: list[Row] = []
    rejections: list[Rejection] = []
    try:
        with p.open(newline="", encoding="utf-8-sig") as fh:
            reader = csv.DictReader(fh)
            if reader.fieldnames is None:
                raise InputFileError(str(p), "empty file: no header row")
            missing = [c for c in HEADER if c not in reader.fieldnames]
            if missing:
                raise InputFileError(str(p), f"missing column(s): {', '.join(missing)}")
            for raw in reader:
                line = reader.line_num
                try:
                    rows.append(_parse_row(raw, line))
                except _RowError as exc:
                    rejections.append(Rejection(line, str(exc)))
    except OSError as exc:
        raise InputFileError(str(p), f"cannot read: {exc.strerror or exc}") from exc
    except UnicodeDecodeError as exc:
        raise InputFileError(str(p), f"not valid UTF-8: {exc.reason}") from exc
    except csv.Error as exc:
        raise InputFileError(str(p), f"malformed CSV near line {reader.line_num}: {exc}") from exc
    return rows, rejections


_CONTROL_RE = re.compile(r"[\x00-\x1f\x7f]")


def _text_field(value: str, name: str) -> str:
    """Reject control characters (CR, LF, NUL, tab…) in free text.

    Keeps a value from breaking out of a generated SQL comment line. Quoted
    literals are escaped separately by :mod:`tenant_onboard.sql`.
    """
    if _CONTROL_RE.search(value):
        raise _RowError(f"{name} contains a control character")
    return value


def comment_safe(value: str) -> str:
    """Make text safe for a single-line SQL comment (defence in depth)."""
    return _CONTROL_RE.sub(" ", value)


def _parse_row(raw: dict[str, str | None], line: int) -> Row:
    def field(name: str) -> str:
        value = raw.get(name)
        if value is None or not value.strip():
            raise _RowError(f"missing {name}")
        return value.strip()

    def optional(name: str) -> str:
        return _text_field((raw.get(name) or "").strip(), name)

    try:
        when = date.fromisoformat(field("date"))
    except ValueError as exc:
        raise _RowError(f"date is not YYYY-MM-DD: {raw.get('date')!r}") from exc
    vendor = _text_field(field("vendor"), "vendor")
    reference = _text_field(field("reference"), "reference")
    amount_text = field("amount_usd")
    try:
        amount = Decimal(amount_text)
    except InvalidOperation as exc:
        raise _RowError(f"amount_usd is not a number: {amount_text!r}") from exc
    if not amount.is_finite():
        raise _RowError(f"amount_usd is not finite: {amount_text!r}")
    status = field("status")
    if status not in LOADABLE_STATUSES and not status.startswith(SKIP_STATUS_PREFIXES):
        raise _RowError(f"unknown status {status!r}")
    return Row(
        line=line, when=when, vendor=vendor, reference=reference,
        description=optional("description"), amount=amount, status=status,
        attribution=optional("attribution"), notes=optional("notes"),
    )


# ── classification and gates ──────────────────────────────────────────────

def classify(rows: Iterable[Row], rejections: Iterable[Rejection] = ()) -> Plan:
    """Apply status, hold, attribution and sign rules to parsed rows."""
    loadable: list[Loadable] = []
    skipped: list[Skip] = []
    tbd: list[Row] = []
    rejected: list[Rejection] = list(rejections)

    def skip(row: Row, reason: str) -> None:
        skipped.append(Skip(row.line, row.vendor, row.reference, row.amount, reason))

    for row in rows:
        if row.vendor.startswith(HELD_VENDOR_PREFIXES):
            skip(row, "held: owner must confirm before loading")
            continue
        if row.status not in LOADABLE_STATUSES:
            skip(row, f"status {row.status!r} is not loadable")
            continue
        if row.amount <= 0:
            rejected.append(Rejection(row.line, "amount_usd must be positive for a loadable row"))
            continue
        if row.attribution == "TBD":
            tbd.append(row)
            continue
        match = _PCT_RE.match(row.attribution)
        if match is None:
            rejected.append(Rejection(row.line, f"attribution is not a percentage: {row.attribution!r}"))
            continue
        pct = Decimal(match.group(1))
        if pct > 100:
            rejected.append(Rejection(row.line, f"attribution above 100%: {row.attribution!r}"))
            continue
        if pct == 0:
            skip(row, "attribution 0%")
            continue
        fraction = pct / Decimal(100)
        company = (row.amount * fraction).quantize(CENT, rounding=ROUND_HALF_UP)
        if company <= 0:
            rejected.append(Rejection(row.line, "company share rounds to zero"))
            continue
        loadable.append(Loadable(row, fraction, company))
    return Plan(tuple(loadable), tuple(skipped), tuple(tbd), tuple(rejected))


def reconcile(loadable: Iterable[Loadable], tbd: Iterable[Row],
              expected: Decimal = V2_CANDIDATE_TOTAL) -> Reconciliation:
    """Total candidate original amounts against ``expected``. Never adjusts."""
    items = list(loadable)
    pending = list(tbd)
    loadable_total = sum((i.row.amount for i in items), Decimal("0"))
    return Reconciliation(
        expected=expected,
        candidate_total=loadable_total + sum((r.amount for r in pending), Decimal("0")),
        loadable_total=loadable_total,
        company_total=sum((i.company_amount for i in items), Decimal("0")),
    )


# ── entries and SQL ───────────────────────────────────────────────────────

def build_entries(loadable: Iterable[Loadable]) -> list[Entry]:
    """One entry per loadable row. ``entry_number`` is the idempotency key.

    Raises:
        RenderError: A reference yields no usable key, or two rows collide.
    """
    out: list[Entry] = []
    seen: dict[str, int] = {}
    for item in loadable:
        row = item.row
        token = re.sub(r"[^A-Za-z0-9]", "", row.reference).upper()
        if not token:
            raise RenderError(f"line {row.line}: reference {row.reference!r} gives no entry key")
        number = f"{ENTRY_PREFIX}{row.when.strftime('%Y%m%d')}-{token}"
        if number in seen:
            raise RenderError(f"line {row.line}: entry key {number} already used by line {seen[number]}")
        seen[number] = row.line
        narration = (
            f"Start-up cost {row.when.isoformat()} {row.vendor}: {row.description}. "
            f"Original ${row.amount:.2f} at {_pct_text(item.fraction)}% company share "
            f"${item.company_amount:.2f}. Reimbursable liability (Operating Agreement §4.5), not capital."
        )
        out.append(Entry(row.line, row.when, row.vendor, row.reference, row.amount,
                         item.fraction, item.company_amount, number, narration))
    return out


def _pct_text(fraction: Decimal) -> str:
    return format((fraction * 100).normalize(), "f")


def render_sql(entries: list[Entry], company: Company, owed_key: str) -> str:
    """Render the loader SQL: one party row, then one atomic CTE per entry.

    Re-running is safe: the entry insert is ``ON CONFLICT (entry_number) DO
    NOTHING`` and the lines only insert when the entry was new.

    Raises:
        RenderError: The owed member is unknown or has no real name.
    """
    member = owed_member(company, owed_key)
    tenant = sql.uuid_lit(company.tenant_id)
    party_id = sql.uuid_lit(uuid.uuid5(uuid.UUID(company.tenant_id), f"startup-party:{owed_key}"))
    year, month = company.books.open_periods()[0]
    period = (f"(SELECT id FROM accounting_periods WHERE tenant_id = {tenant} "
              f"AND year = {sql.integer(year)} AND month = {sql.integer(month)})")

    def account(code: str) -> str:
        return f"(SELECT id FROM accounts WHERE tenant_id = {tenant} AND code = {sql.text(code)})"

    out = [
        "-- Generated by tenant-onboard startup-load. Do not edit by hand.",
        "-- Apply after the tenant seed (001-004). Entries load as drafts.",
        "BEGIN;",
        "INSERT INTO parties (id, tenant_id, name, party_type, is_active)",
        f"VALUES ({party_id}, {tenant}, {sql.text(member.name)}, 'supplier', true)",
        "ON CONFLICT (id) DO NOTHING;",
        "",
    ]
    for e in entries:
        amount = f"{e.company_amount:.2f}"
        debit_desc = sql.text(f"Start-up cost {e.reference}")
        credit_desc = sql.text(f"Reimbursable to {member.name}")
        out.extend([
            f"-- line {e.line}: {comment_safe(e.vendor)} {comment_safe(e.reference)} ({e.when.isoformat()})",
            "WITH je AS (",
            "  INSERT INTO journal_entries (tenant_id, period_id, entry_number, reference, narration, status)",
            f"  VALUES ({tenant}, {period}, {sql.text(e.entry_number)}, {sql.text(e.reference)}, "
            f"{sql.text(e.narration)}, 'draft')",
            "  ON CONFLICT (entry_number) DO NOTHING",
            "  RETURNING id",
            ")",
            "INSERT INTO journal_lines (journal_id, account_id, debit_amount, credit_amount, currency, description, party_id)",
            f"SELECT je.id, {account(DEBIT_ACCOUNT)}, {amount}, 0, 'USD', {debit_desc}, NULL::uuid FROM je",
            "UNION ALL",
            f"SELECT je.id, {account(PAYABLE_ACCOUNT)}, 0, {amount}, 'USD', {credit_desc}, {party_id}::uuid FROM je;",
            "",
        ])
    out.append("COMMIT;")
    return "\n".join(out) + "\n"


def render_manifest(entries: list[Entry]) -> str:
    """CSV manifest: original amount and company share side by side."""
    buf = io.StringIO()
    writer = csv.writer(buf, lineterminator="\n")
    writer.writerow(["line", "date", "vendor", "reference", "entry_number",
                     "amount_usd", "company_pct", "company_usd"])
    for e in entries:
        writer.writerow([e.line, e.when.isoformat(), e.vendor, e.reference, e.entry_number,
                         f"{e.amount:.2f}", _pct_text(e.fraction), f"{e.company_amount:.2f}"])
    return buf.getvalue()


def owed_member(company: Company, owed_key: str) -> Member:
    """Return the entity member owed the reimbursement, with a real name.

    Raises:
        RenderError: No entity block, unknown key, or the member has no name.
    """
    if company.entity is None:
        raise RenderError("company has no entity block; startup loads need entity.members")
    for member in company.entity.members:
        if member.key == owed_key:
            if not member.name:
                raise RenderError(
                    f"member {owed_key!r} has no name; supply entity.members.{owed_key}.name in company.local.yaml"
                )
            return member
    keys = ", ".join(m.key for m in company.entity.members)
    raise RenderError(f"no member {owed_key!r} in entity.members (known: {keys})")
