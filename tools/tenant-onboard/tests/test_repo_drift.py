"""Repo-aware checks: generator stays in lockstep with the rest of Thittam.

Skipped automatically when the tool is used outside the Thittam repo.
"""

from __future__ import annotations

import re

import pytest

from conftest import REPO_ROOT
from tenant_onboard.company import check_against_vertical, load_company
from tenant_onboard.render import SYSTEM_ROLES, render_all
from tenant_onboard.vertical import load_vertical, render_migration

TEMPLATE = REPO_ROOT / "seeds" / "template" / "new-tenant" / "001_tenant.sql"
US_YAML_REL = "pkg/vertical/configs/software-development-us.yaml"
US_MIGRATION = REPO_ROOT / "migrations" / "shared" / "007_seed_software_development_us_vertical"
KAUNDINYA = REPO_ROOT / "seeds" / "tenants" / "kaundinya-labs" / "company.yaml"

pytestmark = pytest.mark.skipif(not TEMPLATE.exists(), reason="not running inside the Thittam repo")


def _template_roles() -> dict[str, tuple[str, ...]]:
    text = TEMPLATE.read_text()
    roles = {}
    for name, perms in re.findall(r"\('<TENANT_UUID>', '(\w+)',\s*ARRAY\[(.*?)\]", text, re.DOTALL):
        roles[name] = tuple(re.findall(r"'([^']+)'", perms))
    return roles


def test_system_roles_match_new_tenant_template():
    assert dict(SYSTEM_ROLES) == _template_roles()


def test_us_vertical_migration_is_current():
    vertical = load_vertical(REPO_ROOT / US_YAML_REL)
    up, down = render_migration(vertical, US_YAML_REL)
    assert US_MIGRATION.with_suffix(".up.sql").read_text() == up, "run `make vertical-migrations`"
    assert US_MIGRATION.with_suffix(".down.sql").read_text() == down, "run `make vertical-migrations`"


def test_us_vertical_default_account_codes_exist_in_coa():
    vertical = load_vertical(REPO_ROOT / US_YAML_REL)
    codes = {a.code for a in vertical.accounts}
    cfg = vertical.config
    refs = [c["default_account_code"] for c in cfg["expense_categories"]]
    refs += [c["default_account_code"] for c in cfg["budget_categories"]]
    refs += [li["account_code"] for t in cfg["budget_templates"] for li in t["line_items"]]
    assert set(refs) <= codes


def test_kaundinya_config_with_mock_private_details(tmp_path):
    """The committed config is complete apart from the private fields."""
    local = tmp_path / "company.local.yaml"
    local.write_text(
        "address:\n  line1: 1 Mock Street\n  city: Mocktown\n  postal_code: '48000'\n"
        "admin:\n  email: owner@example.test\n"
    )
    company = load_company(KAUNDINYA, local)
    vertical = load_vertical(REPO_ROOT / "pkg" / "vertical" / "configs" / f"{company.vertical_id}.yaml")
    check_against_vertical(company, vertical)

    assert company.name == "Kaundinya Labs LLC"
    assert [p.slug for p in company.products] == [
        "mentible", "kathai-chithiram", "studybuddy-ondemand", "company-overhead",
    ]
    assert company.books.open_periods() == [(2026, 10), (2026, 11), (2026, 12)]
    files = render_all(company, vertical)
    assert "REPLACE_ME" not in "".join(files.values())


def test_kaundinya_committed_config_keeps_private_fields_out():
    text = KAUNDINYA.read_text()
    for field in ("line1", "city", "postal_code", "email"):
        assert re.search(rf"^\s+{field}: REPLACE_ME\s*$", text, re.M), f"{field} must stay REPLACE_ME"


def test_local_override_is_gitignored():
    gitignore = (REPO_ROOT / ".gitignore").read_text()
    assert "seeds/tenants/*/company.local.yaml" in gitignore
