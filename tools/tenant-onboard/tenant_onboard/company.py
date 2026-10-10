"""Company (tenant) configuration: loading, merging and validation.

A tenant is described by a committed ``company.yaml`` plus an optional,
git-ignored ``company.local.yaml`` beside it. The local file is deep-merged on
top so private details — street address, login email — never have to be
committed to a public repository. Any string still containing the marker
``REPLACE_ME`` after the merge is reported as missing.

Validation mirrors the database constraints in ``migrations/iam`` and
``migrations/project`` so problems surface before ``psql`` runs.
"""

from __future__ import annotations

import copy
import re
import uuid
from dataclasses import dataclass
from datetime import date
from pathlib import Path
from typing import Any

from .errors import CompanyConfigError, Issue
from .vertical import Vertical
from .yamlio import load_yaml_mapping

PLACEHOLDER = "REPLACE_ME"
PLANS = ("starter", "professional", "enterprise")

# productions.status is CHECK-constrained to the movie lifecycle (see
# docs/multi-tenancy.md §7). Vertical stages are mapped onto it until that
# constraint becomes vertical-aware. Stages missing here must be mapped via
# `stage_status_map` in company.yaml.
PRODUCTION_STATUSES = (
    "development", "pre_production", "production", "post_production", "released", "archived",
)
DEFAULT_STAGE_STATUS: dict[str, str] = {
    "discovery": "development",
    "design": "pre_production",
    "build": "production",
    "beta": "post_production",
    "launched": "released",
    "maintenance": "released",
    "sunset": "archived",
}

_SLUG_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
_EMAIL_RE = re.compile(r"^[^@\s]+@[^@\s]+\.[A-Za-z]{2,}$")
_US_ZIP_RE = re.compile(r"^\d{5}(?:-\d{4})?$")


@dataclass(frozen=True)
class Address:
    """Registered business address (drives currency/billing defaults)."""

    line1: str
    line2: str | None
    city: str
    postal_code: str
    country: str


@dataclass(frozen=True)
class Admin:
    """First user; receives the tenant-wide ``super_admin`` role."""

    id: str
    email: str
    display_name: str


@dataclass(frozen=True)
class Product:
    """One row in ``productions`` (labelled "Product" by the vertical)."""

    slug: str
    title: str
    description: str | None
    stage: str
    start_date: date | None
    end_date: date | None


@dataclass(frozen=True)
class Books:
    """Accounting period setup.

    Attributes:
        opening_date: First day transactions are recorded.
        fiscal_year_end_month: Month (1-12) the fiscal year ends.
    """

    opening_date: date
    fiscal_year_end_month: int

    def open_periods(self) -> list[tuple[int, int]]:
        """Return ``(year, month)`` pairs from the opening month through the
        end of the fiscal year that contains the opening date."""
        y, m = self.opening_date.year, self.opening_date.month
        end_y = y if m <= self.fiscal_year_end_month else y + 1
        out: list[tuple[int, int]] = []
        while (y, m) <= (end_y, self.fiscal_year_end_month):
            out.append((y, m))
            y, m = (y + 1, 1) if m == 12 else (y, m + 1)
        return out


@dataclass(frozen=True)
class Company:
    """A fully validated tenant definition."""

    tenant_id: str
    name: str
    slug: str
    plan: str
    vertical_id: str
    currency: str
    address: Address
    admin: Admin
    books: Books
    products: tuple[Product, ...]
    stage_status_map: dict[str, str]

    def status_for(self, stage: str) -> str:
        """Map a vertical stage id to an allowed ``productions.status``."""
        return self.stage_status_map[stage]


def deep_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    """Recursively merge ``override`` onto a copy of ``base``.

    Mappings merge key by key; any other value (including lists) in
    ``override`` replaces the base value. Inputs are not modified.
    """
    out = copy.deepcopy(base)
    for k, v in override.items():
        if isinstance(v, dict) and isinstance(out.get(k), dict):
            out[k] = deep_merge(out[k], v)
        else:
            out[k] = copy.deepcopy(v)
    return out


def load_company(path: str | Path, local_override: str | Path | None = None) -> Company:
    """Load ``company.yaml`` (and its local override, if present) and validate.

    Args:
        path: The committed company config.
        local_override: Override file. Defaults to ``company.local.yaml`` in
            the same directory; silently skipped when that default is absent.
            An explicitly passed path must exist.

    Returns:
        The validated :class:`Company`.

    Raises:
        InputFileError: A file cannot be read or parsed.
        CompanyConfigError: With every validation issue found.
    """
    p = Path(path)
    data = load_yaml_mapping(p)
    if local_override is None:
        default_local = p.with_name("company.local.yaml")
        if default_local.exists():
            data = deep_merge(data, load_yaml_mapping(default_local))
    else:
        data = deep_merge(data, load_yaml_mapping(local_override))
    return parse_company(data, source=str(p))


def parse_company(data: dict[str, Any], source: str = "<memory>") -> Company:
    """Validate an already-merged company mapping.

    Raises:
        CompanyConfigError: With every validation issue found.
    """
    issues: list[Issue] = []
    _find_placeholders(data, "", issues)

    t = _section(data, "tenant", issues)
    a = _section(data, "address", issues)
    ad = _section(data, "admin", issues)
    b = _section(data, "books", issues)

    tenant_id = _uuid(t, "tenant.id", issues)
    name = _str(t, "name", "tenant.name", issues)
    if name is not None:
        name = " ".join(name.split())  # mirrors migration 018 whitespace collapse
        if len(name) > 200:
            issues.append(Issue("tenant.name", "must be at most 200 characters"))
    slug = _str(t, "slug", "tenant.slug", issues)
    if slug is not None and not _SLUG_RE.match(slug):
        issues.append(Issue("tenant.slug", "must be lowercase words joined by single hyphens"))
    plan = _str(t, "plan", "tenant.plan", issues)
    if plan is not None and plan not in PLANS:
        issues.append(Issue("tenant.plan", f"must be one of {', '.join(PLANS)}"))
    vertical_id = _str(t, "vertical", "tenant.vertical", issues)
    currency = _str(t, "primary_currency", "tenant.primary_currency", issues)
    if currency is not None and not re.fullmatch(r"[A-Z]{3}", currency):
        issues.append(Issue("tenant.primary_currency", "must be a 3-letter upper-case ISO 4217 code"))

    country = _str(a, "country", "address.country", issues)
    if country is not None and not re.fullmatch(r"[A-Z]{2}", country):
        issues.append(Issue("address.country", "must be a 2-letter upper-case ISO 3166-1 code"))
    line1 = _str(a, "line1", "address.line1", issues)
    line2 = _str(a, "line2", "address.line2", issues, required=False)
    city = _str(a, "city", "address.city", issues)
    postal = _str(a, "postal_code", "address.postal_code", issues)
    if postal is not None and country == "US" and not _US_ZIP_RE.match(postal):
        issues.append(Issue("address.postal_code", "US ZIP must be 12345 or 12345-6789 (quote it in YAML)"))

    admin_id = _uuid(ad, "admin.id", issues)
    if admin_id is not None and admin_id == tenant_id:
        issues.append(Issue("admin.id", "must differ from tenant.id"))
    email = _str(ad, "email", "admin.email", issues)
    if email is not None and not _EMAIL_RE.match(email):
        issues.append(Issue("admin.email", "not a valid email address"))
    display = _str(ad, "display_name", "admin.display_name", issues)

    opening = b.get("opening_date") if b else None
    if not isinstance(opening, date):
        issues.append(Issue("books.opening_date", "required date (YYYY-MM-DD, unquoted)"))
        opening = None
    fye = b.get("fiscal_year_end_month", 12) if b else 12
    if isinstance(fye, bool) or not isinstance(fye, int) or not 1 <= fye <= 12:
        issues.append(Issue("books.fiscal_year_end_month", "must be an integer 1-12"))
        fye = 12

    stage_map = dict(DEFAULT_STAGE_STATUS)
    raw_map = data.get("stage_status_map") or {}
    if not isinstance(raw_map, dict):
        issues.append(Issue("stage_status_map", "must be a mapping of stage -> status"))
    else:
        for stage, status in raw_map.items():
            if status not in PRODUCTION_STATUSES:
                issues.append(Issue(f"stage_status_map.{stage}", f"must be one of {', '.join(PRODUCTION_STATUSES)}"))
            else:
                stage_map[str(stage)] = status

    products = _parse_products(data.get("products"), issues)

    if issues:
        raise CompanyConfigError(source, issues)

    return Company(
        tenant_id=tenant_id, name=name, slug=slug, plan=plan, vertical_id=vertical_id,
        currency=currency,
        address=Address(line1=line1, line2=line2, city=city, postal_code=postal, country=country),
        admin=Admin(id=admin_id, email=email.lower(), display_name=display),
        books=Books(opening_date=opening, fiscal_year_end_month=fye),
        products=tuple(products), stage_status_map=stage_map,
    )


def check_against_vertical(company: Company, vertical: Vertical, source: str = "<company>") -> None:
    """Cross-check a company config against the vertical it binds to.

    Raises:
        CompanyConfigError: If the vertical id differs, a product uses a stage
            the vertical does not define, or a stage has no status mapping.
    """
    issues: list[Issue] = []
    if company.vertical_id != vertical.id:
        issues.append(Issue("tenant.vertical", f"is {company.vertical_id!r} but vertical file defines {vertical.id!r}"))
    for i, p in enumerate(company.products):
        if p.stage not in vertical.stage_ids:
            issues.append(Issue(f"products[{i}].stage", f"{p.stage!r} is not a stage of {vertical.id} ({', '.join(vertical.stage_ids)})"))
        elif p.stage not in company.stage_status_map:
            issues.append(Issue(f"products[{i}].stage", f"no productions.status mapping for {p.stage!r}; add it to stage_status_map"))
    if issues:
        raise CompanyConfigError(source, issues)


# ── helpers ────────────────────────────────────────────────────────────────

def _find_placeholders(node: Any, path: str, issues: list[Issue]) -> None:
    """Report every string value that still contains ``REPLACE_ME``."""
    if isinstance(node, dict):
        for k, v in node.items():
            _find_placeholders(v, f"{path}.{k}" if path else str(k), issues)
    elif isinstance(node, list):
        for i, v in enumerate(node):
            _find_placeholders(v, f"{path}[{i}]", issues)
    elif isinstance(node, str) and PLACEHOLDER in node:
        issues.append(Issue(path, "still set to REPLACE_ME — fill it in company.local.yaml"))


def _section(data: dict[str, Any], key: str, issues: list[Issue]) -> dict[str, Any]:
    val = data.get(key)
    if not isinstance(val, dict):
        issues.append(Issue(key, "required section (mapping)"))
        return {}
    return val


def _str(sec: dict[str, Any], key: str, field: str, issues: list[Issue], required: bool = True) -> str | None:
    val = sec.get(key)
    if val is None:
        if required:
            issues.append(Issue(field, "required"))
        return None
    if not isinstance(val, str):
        issues.append(Issue(field, f"must be a string, got {type(val).__name__}"))
        return None
    val = val.strip()
    if not val:
        if required:
            issues.append(Issue(field, "must not be empty"))
        return None
    if PLACEHOLDER in val:
        return None  # already reported by _find_placeholders
    return val


def _uuid(sec: dict[str, Any], field: str, issues: list[Issue]) -> str | None:
    raw = _str(sec, field.split(".")[-1], field, issues)
    if raw is None:
        return None
    try:
        return str(uuid.UUID(raw))
    except ValueError:
        issues.append(Issue(field, "must be a UUID"))
        return None


def _parse_products(raw: Any, issues: list[Issue]) -> list[Product]:
    if not isinstance(raw, list) or not raw:
        issues.append(Issue("products", "required non-empty list"))
        return []
    out: list[Product] = []
    seen_slugs: set[str] = set()
    for i, row in enumerate(raw):
        where = f"products[{i}]"
        if not isinstance(row, dict):
            issues.append(Issue(where, "must be a mapping"))
            continue
        before = len(issues)
        slug = _str(row, "slug", f"{where}.slug", issues)
        if slug is not None:
            if not _SLUG_RE.match(slug):
                issues.append(Issue(f"{where}.slug", "must be lowercase words joined by single hyphens"))
            elif slug in seen_slugs:
                issues.append(Issue(f"{where}.slug", f"duplicate slug {slug!r}"))
            seen_slugs.add(slug)
        title = _str(row, "title", f"{where}.title", issues)
        desc = _str(row, "description", f"{where}.description", issues, required=False)
        stage = _str(row, "stage", f"{where}.stage", issues)
        start, end = row.get("start_date"), row.get("end_date")
        for key, val in (("start_date", start), ("end_date", end)):
            if val is not None and not isinstance(val, date):
                issues.append(Issue(f"{where}.{key}", "must be a date (YYYY-MM-DD, unquoted)"))
        if isinstance(start, date) and isinstance(end, date) and end < start:
            issues.append(Issue(f"{where}.end_date", "must not be before start_date"))
        if len(issues) == before:
            out.append(Product(slug=slug, title=title, description=desc, stage=stage,
                               start_date=start, end_date=end))
    return out
