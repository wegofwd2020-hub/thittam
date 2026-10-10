"""Tests for tenant_onboard.render — generated seed SQL content and file output."""

import re

import pytest

from tenant_onboard.errors import OutputError
from tenant_onboard.render import (
    SYSTEM_ROLES,
    render_admin,
    render_all,
    render_ledger,
    render_products,
    render_tenant,
    write_files,
)

TENANT = "'0f6b1c2e-1111-4a5b-9c3d-000000000001'"


def test_every_file_is_transactional_and_stops_on_error(company, vertical):
    for name, body in render_all(company, vertical).items():
        assert "\\set ON_ERROR_STOP on" in body, name
        assert body.count("BEGIN;") == 1 and body.rstrip().endswith("COMMIT;"), name


def test_files_are_named_in_apply_order(company, vertical):
    assert list(render_all(company, vertical)) == [
        "001_tenant.sql", "002_admin.sql", "003_products.sql", "004_ledger.sql",
    ]


class TestTenant:
    def test_escapes_name_and_sets_address(self, company):
        s = render_tenant(company)
        assert "'O''Brien Mock Studio LLC'" in s
        assert "'Testville'" in s and "'49999'" in s and "'US'" in s
        assert "ON CONFLICT (id) DO NOTHING" in s

    def test_binds_vertical(self, company):
        assert f"VALUES ({TENANT}, 'test-studio'," in render_tenant(company)

    def test_seeds_seven_system_roles(self, company):
        s = render_tenant(company)
        assert len(SYSTEM_ROLES) == 7
        for name, _ in SYSTEM_ROLES:
            assert f"({TENANT}, '{name}', ARRAY[" in s
        assert "ON CONFLICT (tenant_id, name) DO NOTHING" in s

    def test_marked_not_demo(self, company):
        assert re.search(r"'USD', false\n\)", render_tenant(company))

    def test_null_line2(self, company_dict):
        from tenant_onboard.company import parse_company

        company_dict["address"]["line2"] = None
        assert ", NULL, 'Testville'" in render_tenant(parse_company(company_dict))


class TestAdmin:
    def test_hash_comes_from_psql_variable_only(self, company):
        s = render_admin(company)
        assert ":'admin_password_hash'" in s
        assert "$2b$" not in s and "$2a$" not in s

    def test_guards_missing_and_malformed_hash(self, company):
        s = render_admin(company)
        assert "\\if :{?admin_password_hash}" in s
        assert "admin_password_hash not supplied" in s
        assert "is not a bcrypt hash" in s

    def test_grants_super_admin_and_verifies(self, company):
        s = render_admin(company)
        # must match the user_roles_unique expression index from iam migration 012
        assert "ON CONFLICT (user_id, role_id, COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid))" in s
        assert "r.name      = 'super_admin'" in s
        assert "super_admin grant missing" in s

    def test_email_escaped_and_lowercased(self, company):
        assert "'owner@example.com', 'Pat O''Brien'" in render_admin(company)


class TestProducts:
    def test_status_mapped_from_stage(self, company):
        s = render_products(company)
        assert "'widget-app', 'Mock product in build.',\n        'production', '2026-10-20', NULL" in s
        assert "'gadget-api', NULL,\n        'development', NULL, NULL" in s

    def test_idempotent(self, company):
        assert render_products(company).count("ON CONFLICT (tenant_id, slug) DO NOTHING") == 2


class TestLedger:
    def test_one_insert_per_account_in_order(self, company, vertical):
        s = render_ledger(company, vertical)
        codes = re.findall(r"VALUES \(\S+, '(\d+)'", s)
        assert codes == [a.code for a in vertical.accounts]

    def test_parent_resolved_by_code(self, company, vertical):
        s = render_ledger(company, vertical)
        assert f"(SELECT id FROM accounts WHERE tenant_id = {TENANT} AND code = '1000')" in s

    def test_root_accounts_have_null_parent(self, company, vertical):
        assert "'Cash & Bank', 'asset', NULL, true)" in render_ledger(company, vertical)

    def test_names_escaped(self, company, vertical):
        assert "'Owner''s Capital'" in render_ledger(company, vertical)

    def test_open_periods(self, company, vertical):
        s = render_ledger(company, vertical)
        for m in (10, 11, 12):
            assert f"({TENANT}, 2026, {m}, 'open')" in s
        assert f"({TENANT}, 2026, 9," not in s
        assert "ON CONFLICT (tenant_id, year, month) DO NOTHING" in s


class TestWriteFiles:
    def test_writes_and_leaves_no_temp_files(self, tmp_path, company, vertical):
        out = tmp_path / "nested" / "seed"
        paths = write_files(render_all(company, vertical), out)
        assert [p.name for p in paths] == sorted(p.name for p in paths)
        assert sorted(f.name for f in out.iterdir()) == [p.name for p in paths]

    def test_overwrites_existing(self, tmp_path):
        write_files({"a.sql": "one"}, tmp_path)
        write_files({"a.sql": "two"}, tmp_path)
        assert (tmp_path / "a.sql").read_text() == "two"

    def test_out_dir_is_a_file(self, tmp_path):
        blocker = tmp_path / "file"
        blocker.write_text("x")
        with pytest.raises(OutputError, match="cannot write"):
            write_files({"a.sql": "x"}, blocker)

    def test_target_is_a_directory(self, tmp_path):
        (tmp_path / "a.sql").mkdir()
        with pytest.raises(OutputError):
            write_files({"a.sql": "x"}, tmp_path)
        assert not (tmp_path / ".a.sql.tmp").exists()
