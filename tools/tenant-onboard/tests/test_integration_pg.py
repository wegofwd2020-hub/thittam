"""End-to-end: real migrations + generated seed SQL on a disposable PostgreSQL.

Runs only when ``THITTAM_TEST_DSN`` points at a database you are happy to
create throwaway schemas in (the test creates and drops its own schema; it
never touches ``public``). Follows the repo rule: never use the
``infra/local`` compose stack for this.

    THITTAM_TEST_DSN=postgresql://postgres@127.0.0.1:55432/thittam_it \\
        python3 -m pytest -m integration
"""

from __future__ import annotations

import os
import shutil
import subprocess
import uuid

import pytest

from conftest import REPO_ROOT
from tenant_onboard.company import parse_company
from tenant_onboard.render import render_all, write_files
from tenant_onboard.vertical import load_vertical

DSN = os.environ.get("THITTAM_TEST_DSN")
SERVICES = ("shared", "iam", "project", "ledger")  # Makefile migrate-all order
# A real bcrypt hash of a throwaway mock password (cost 4) — test data only.
MOCK_HASH = "$2b$04$v6DmppWv7CGx3m6hsh38ueaX0OmyL0oWDiGi84wJU81yHEmL5JG1C"  # "mock-password-123"

pytestmark = [
    pytest.mark.integration,
    pytest.mark.skipif(not DSN, reason="THITTAM_TEST_DSN not set"),
    pytest.mark.skipif(shutil.which("psql") is None, reason="psql not installed"),
    pytest.mark.skipif(not (REPO_ROOT / "migrations").is_dir(), reason="not inside the Thittam repo"),
]


class Psql:
    """Runs psql with search_path pinned to one throwaway schema."""

    def __init__(self, schema: str) -> None:
        self.env = {**os.environ, "PGOPTIONS": f"-c search_path={schema},public"}

    def run(self, *args: str, check: bool = True) -> subprocess.CompletedProcess:
        cmd = ["psql", DSN, "-X", "-q", "-v", "ON_ERROR_STOP=1", *args]
        return subprocess.run(cmd, env=self.env, capture_output=True, text=True, check=check)

    def file(self, path, *extra: str, check: bool = True) -> subprocess.CompletedProcess:
        return self.run(*extra, "-f", str(path), check=check)

    def scalar(self, query: str) -> str:
        return self.run("-At", "-c", query).stdout.strip()


@pytest.fixture(scope="module")
def db():
    schema = f"onboard_it_{uuid.uuid4().hex[:8]}"
    admin = Psql("public")
    admin.run("-c", f"CREATE SCHEMA {schema}")
    p = Psql(schema)
    try:
        for svc in SERVICES:
            for f in sorted((REPO_ROOT / "migrations" / svc).glob("*.up.sql")):
                res = p.file(f, check=False)
                if res.returncode != 0:
                    pytest.fail(f"migration {f.name} failed:\n{res.stderr}")
        yield p
    finally:
        admin.run("-c", f"DROP SCHEMA {schema} CASCADE", check=False)


@pytest.fixture(scope="module")
def seed_dir(tmp_path_factory):
    import yaml

    from conftest import FIXTURES

    data = yaml.safe_load((FIXTURES / "company_valid.yaml").read_text())
    data["tenant"]["vertical"] = "software-development-us"
    data["products"][0]["stage"] = "build"
    data["products"][1]["stage"] = "maintenance"
    data["entity"] = {"tax_classification": "disregarded_schedule_c",
                      "members": {"owner": {"label": "Owner", "ownership_pct": 100}}}
    company = parse_company(data)
    vertical = load_vertical(REPO_ROOT / "pkg" / "vertical" / "configs" / "software-development-us.yaml")
    out = tmp_path_factory.mktemp("seed")
    write_files(render_all(company, vertical), out)
    return out, company, vertical


def _apply(db: Psql, out, hash_value: str | None = MOCK_HASH, check: bool = True):
    results = []
    for f in sorted(out.glob("*.sql")):
        extra = ("-v", f"admin_password_hash={hash_value}") if hash_value is not None else ()
        results.append(db.file(f, *extra, check=check))
    return results


def test_vertical_registered_by_migration(db):
    assert db.scalar("SELECT name FROM vertical_definitions WHERE id='software-development-us'") == "Software Studio (US)"
    assert db.scalar(
        "SELECT config->'entity_labels'->>'project' FROM vertical_definitions WHERE id='software-development-us'"
    ) == "Product"


def test_admin_without_hash_fails_cleanly(db, seed_dir):
    out, *_ = seed_dir
    db.file(out / "001_tenant.sql", "-v", "x=1")
    res = db.file(out / "002_admin.sql", check=False)
    assert res.returncode != 0 and "admin_password_hash not supplied" in res.stderr
    res = db.file(out / "002_admin.sql", "-v", "admin_password_hash=plaintext", check=False)
    assert res.returncode != 0 and "not a bcrypt hash" in res.stderr


def test_full_seed_applies_and_is_idempotent(db, seed_dir):
    out, company, vertical = seed_dir
    _apply(db, out)
    _apply(db, out)  # second run must be a no-op, not an error

    t = company.tenant_id
    assert db.scalar(f"SELECT name FROM tenants WHERE id='{t}'") == "O'Brien Mock Studio LLC"
    assert db.scalar(f"SELECT vertical_id FROM tenant_verticals WHERE tenant_id='{t}'") == "software-development-us"
    assert db.scalar(f"SELECT count(*) FROM roles WHERE tenant_id='{t}' AND is_system") == "7"
    assert db.scalar(
        f"SELECT r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id='{company.admin.id}'"
    ) == "super_admin"
    assert db.scalar(f"SELECT password_hash FROM users WHERE id='{company.admin.id}'") == MOCK_HASH
    assert db.scalar(
        f"SELECT string_agg(slug||'='||status, ',' ORDER BY slug) FROM productions WHERE tenant_id='{t}'"
    ) == "gadget-api=released,widget-app=production"
    assert db.scalar(f"SELECT count(*) FROM accounts WHERE tenant_id='{t}'") == str(len(vertical.accounts) + 2)
    assert db.scalar(
        f"SELECT p.code FROM accounts c JOIN accounts p ON p.id=c.parent_id WHERE c.tenant_id='{t}' AND c.code='3200'"
    ) == "3000"
    assert db.scalar(
        f"SELECT p.code FROM accounts c JOIN accounts p ON p.id=c.parent_id WHERE c.tenant_id='{t}' AND c.code='6510'"
    ) == "6500"
    assert db.scalar(
        f"SELECT string_agg(month::text, ',' ORDER BY month) FROM accounting_periods WHERE tenant_id='{t}' AND year=2026"
    ) == "10,11,12"


def test_partial_existing_accounts_still_resolve_parents(db, seed_dir):
    """Accounts already created by the ledger service (different ids) are reused."""
    out, company, _ = seed_dir
    t2 = "0f6b1c2e-9999-4a5b-9c3d-000000000009"
    sql_text = (out / "004_ledger.sql").read_text().replace(company.tenant_id, t2)
    pre = out.parent / "pre_ledger.sql"
    pre.write_text(sql_text)
    db.run("-c", f"INSERT INTO accounts (tenant_id, code, name, account_type) VALUES ('{t2}','6000','Opex (pre-existing)','expense')")
    db.file(pre)
    assert db.scalar(
        f"SELECT p.name FROM accounts c JOIN accounts p ON p.id=c.parent_id WHERE c.tenant_id='{t2}' AND c.code='6100'"
    ) == "Opex (pre-existing)"


# ── partnership books (add-partnership-books) ──────────────────────────────

LEGACY_V1_0 = (  # the single-owner rows software-development-us v1.0.0 seeded
    ("3000", "Owner''s Capital", "equity"),
    ("3100", "Owner''s Contributions", "equity"),
    ("3200", "Owner''s Draws", "equity"),
    ("2300", "Due to Owner (reimbursable)", "liability"),
)


def _partnership_ledger(tmp_path, tenant_id: str):
    """004_ledger.sql for the mock partnership, bound to the US vertical."""
    import yaml

    from conftest import FIXTURES

    data = yaml.safe_load((FIXTURES / "company_partnership.yaml").read_text())
    data["tenant"]["id"] = tenant_id
    data["tenant"]["vertical"] = "software-development-us"
    company = parse_company(data)
    vertical = load_vertical(REPO_ROOT / "pkg" / "vertical" / "configs" / "software-development-us.yaml")
    path = tmp_path / f"ledger_{tenant_id[-4:]}.sql"
    path.write_text(render_all(company, vertical)["004_ledger.sql"])
    return path, vertical


def _seed_legacy(db: Psql, tenant_id: str, rename_3000_to: str | None = None) -> None:
    rows = []
    for code, name, kind in LEGACY_V1_0:
        if code == "3000" and rename_3000_to:
            name = rename_3000_to
        rows.append(f"('{tenant_id}', '{code}', '{name}', '{kind}')")
    db.run("-c", "INSERT INTO accounts (tenant_id, code, name, account_type) VALUES " + ", ".join(rows))
    db.run("-c", f"UPDATE accounts c SET parent_id = p.id FROM accounts p WHERE c.tenant_id='{tenant_id}' "
                 f"AND p.tenant_id='{tenant_id}' AND p.code='3000' AND c.code IN ('3100','3200')")


def test_update_migration_registers_v1_1(db):
    assert db.scalar("SELECT version FROM vertical_definitions WHERE id='software-development-us'") == "1.1.0"
    assert db.scalar(
        "SELECT count(*) FROM vertical_definitions, jsonb_array_elements(config->'report_definitions') r "
        "WHERE id='software-development-us' AND r->>'id'='tax_return_rollup'") == "1"


def test_update_migration_down_restores_v1_0_and_up_reapplies(db):
    down = REPO_ROOT / "migrations" / "shared" / "008_update_software_development_us_vertical_v1_1.down.sql"
    db.file(down)
    assert db.scalar("SELECT version FROM vertical_definitions WHERE id='software-development-us'") == "1.0.0"
    db.file(down.with_name(down.name.replace(".down.", ".up.")))
    assert db.scalar("SELECT version FROM vertical_definitions WHERE id='software-development-us'") == "1.1.0"


def test_partnership_upgrade_from_single_owner_chart(db, tmp_path):
    t = "0f6b1c2e-7777-4a5b-9c3d-000000000007"
    _seed_legacy(db, t)
    ledger, vertical = _partnership_ledger(tmp_path, t)
    db.file(ledger)
    before = db.scalar(f"SELECT string_agg(code||name||is_active::text, ',' ORDER BY code) FROM accounts WHERE tenant_id='{t}'")
    db.file(ledger)  # re-run is a no-op
    assert db.scalar(f"SELECT string_agg(code||name||is_active::text, ',' ORDER BY code) FROM accounts WHERE tenant_id='{t}'") == before

    q = lambda code: db.scalar(f"SELECT name||'|'||is_active FROM accounts WHERE tenant_id='{t}' AND code='{code}'")
    assert q("3000") == "Equity|true"
    assert q("2300") == "Due to Owners (reimbursable)|true"
    assert q("3100") == "Owner's Contributions|false"  # history kept, not deleted
    assert q("3200") == "Owner's Draws|false"
    assert q("3020") == "Capital — Robin O'Hara|true"
    assert db.scalar(f"SELECT p.code FROM accounts c JOIN accounts p ON p.id=c.parent_id "
                     f"WHERE c.tenant_id='{t}' AND c.code='3110'") == "3000"
    assert db.scalar(f"SELECT p.code FROM accounts c JOIN accounts p ON p.id=c.parent_id "
                     f"WHERE c.tenant_id='{t}' AND c.code='6052'") == "6050"
    active = int(db.scalar(f"SELECT count(*) FROM accounts WHERE tenant_id='{t}' AND is_active"))
    assert active == len(vertical.accounts) + 8


def test_partnership_upgrade_respects_names_changed_in_app(db, tmp_path):
    t = "0f6b1c2e-8888-4a5b-9c3d-000000000008"
    _seed_legacy(db, t, rename_3000_to="Partners Equity")
    ledger, _ = _partnership_ledger(tmp_path, t)
    db.file(ledger)
    assert db.scalar(f"SELECT name FROM accounts WHERE tenant_id='{t}' AND code='3000'") == "Partners Equity"
