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
    assert db.scalar(f"SELECT count(*) FROM accounts WHERE tenant_id='{t}'") == str(len(vertical.accounts))
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
