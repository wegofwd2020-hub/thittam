"""Tests for tenant_onboard.cli — commands, exit codes and messages."""

import pytest

from tenant_onboard import cli


@pytest.fixture
def run(capsys):
    def _run(*argv):
        code = cli.main(list(argv))
        out, err = capsys.readouterr()
        return code, out, err

    return _run


class TestValidateAndTenant:
    def test_validate_ok(self, run, company_path, vertical_path):
        code, out, _ = run("validate", "--config", str(company_path), "--vertical", str(vertical_path))
        assert code == cli.EXIT_OK
        assert "2 product(s)" in out and "6 accounts" in out and "3 open period(s)" in out

    def test_tenant_writes_four_files(self, run, company_path, vertical_path, tmp_path):
        code, out, _ = run("tenant", "--config", str(company_path), "--vertical", str(vertical_path),
                           "--out", str(tmp_path / "out"))
        assert code == cli.EXIT_OK
        assert out.count("wrote ") == 4
        assert (tmp_path / "out" / "004_ledger.sql").exists()

    def test_invalid_config_exit_2(self, run, write_yaml, vertical_path):
        bad = write_yaml("company.yaml", {"tenant": {"id": "x"}})
        code, _, err = run("validate", "--config", str(bad), "--vertical", str(vertical_path))
        assert code == cli.EXIT_INPUT
        assert "validation issue(s)" in err and "tenant.id" in err

    def test_missing_file_exit_2(self, run, tmp_path, vertical_path):
        code, _, err = run("validate", "--config", str(tmp_path / "nope.yaml"), "--vertical", str(vertical_path))
        assert code == cli.EXIT_INPUT and "file not found" in err

    def test_vertical_mismatch_exit_2(self, run, company_dict, write_yaml, vertical_path):
        company_dict["tenant"]["vertical"] = "other"
        p = write_yaml("company.yaml", company_dict)
        code, _, err = run("validate", "--config", str(p), "--vertical", str(vertical_path))
        assert code == cli.EXIT_INPUT and "tenant.vertical" in err

    def test_vertical_resolved_from_tenant_when_omitted(self, run, company_path, fixtures_dir, tmp_path):
        vdir = tmp_path / "configs"
        vdir.mkdir()
        (vdir / "test-studio.yaml").write_text((fixtures_dir / "vertical_minimal.yaml").read_text())
        code, out, _ = run("validate", "--config", str(company_path), "--verticals-dir", str(vdir))
        assert code == cli.EXIT_OK and "test-studio" in out

    def test_vertical_lookup_missing_exit_2(self, run, company_path, tmp_path):
        code, _, err = run("validate", "--config", str(company_path), "--verticals-dir", str(tmp_path))
        assert code == cli.EXIT_INPUT and "test-studio.yaml" in err and "file not found" in err

    def test_unwritable_output_exit_3(self, run, company_path, vertical_path, tmp_path):
        blocker = tmp_path / "file"
        blocker.write_text("x")
        code, _, err = run("tenant", "--config", str(company_path), "--vertical", str(vertical_path),
                           "--out", str(blocker))
        assert code == cli.EXIT_OUTPUT and "cannot write" in err


class TestVerticalMigration:
    def _args(self, vertical_path, tmp_path):
        return ["--vertical", str(vertical_path), "--source-rel", "fixtures/vertical_minimal.yaml",
                "--up", str(tmp_path / "m.up.sql"), "--down", str(tmp_path / "m.down.sql")]

    def test_generate_then_check(self, run, vertical_path, tmp_path):
        args = self._args(vertical_path, tmp_path)
        assert run("vertical-migration", *args)[0] == cli.EXIT_OK
        code, out, _ = run("check-vertical-migration", *args)
        assert code == cli.EXIT_OK and "OK" in out

    def test_drift_detected(self, run, vertical_path, tmp_path):
        args = self._args(vertical_path, tmp_path)
        run("vertical-migration", *args)
        up = tmp_path / "m.up.sql"
        up.write_text(up.read_text().replace("Test Studio", "Edited By Hand"))
        code, _, err = run("check-vertical-migration", *args)
        assert code == cli.EXIT_CHECK_FAILED
        assert "DRIFT" in err and "Edited By Hand" in err

    def test_missing_migration_is_drift(self, run, vertical_path, tmp_path):
        code, _, err = run("check-vertical-migration", *self._args(vertical_path, tmp_path))
        assert code == cli.EXIT_CHECK_FAILED and "missing" in err


class TestHashPassword:
    def _feed(self, monkeypatch, *answers):
        it = iter(answers)
        monkeypatch.setattr(cli.getpass, "getpass", lambda prompt="": next(it))

    def test_produces_verifiable_bcrypt(self, run, monkeypatch):
        bcrypt = pytest.importorskip("bcrypt")
        self._feed(monkeypatch, "correct horse battery", "correct horse battery")
        code, out, _ = run("hash-password", "--cost", "4")
        h = out.strip()
        assert code == cli.EXIT_OK and h.startswith("$2b$04$") and len(h) == 60
        assert bcrypt.checkpw(b"correct horse battery", h.encode())

    def test_mismatch(self, run, monkeypatch):
        pytest.importorskip("bcrypt")
        self._feed(monkeypatch, "correct horse battery", "different value!!")
        code, _, err = run("hash-password", "--cost", "4")
        assert code == cli.EXIT_INPUT and "do not match" in err

    def test_too_short(self, run, monkeypatch):
        pytest.importorskip("bcrypt")
        self._feed(monkeypatch, "short")
        code, _, err = run("hash-password")
        assert code == cli.EXIT_INPUT and "at least 12" in err

    def test_cost_out_of_range(self, run):
        pytest.importorskip("bcrypt")
        code, _, err = run("hash-password", "--cost", "20")
        assert code == cli.EXIT_INPUT and "--cost" in err


class TestUnexpectedErrors:
    def test_internal_error_exit_70(self, run, monkeypatch, company_path, vertical_path):
        def boom(*_a, **_k):
            raise RuntimeError("kaboom")

        monkeypatch.setattr(cli, "load_vertical", boom)
        code, _, err = run("validate", "--config", str(company_path), "--vertical", str(vertical_path))
        assert code == cli.EXIT_INTERNAL and "kaboom" in err and "--debug" in err

    def test_debug_reraises(self, monkeypatch, company_path, vertical_path):
        monkeypatch.setattr(cli, "load_vertical", lambda *_: (_ for _ in ()).throw(RuntimeError("x")))
        with pytest.raises(RuntimeError):
            cli.main(["--debug", "validate", "--config", str(company_path), "--vertical", str(vertical_path)])

    def test_keyboard_interrupt(self, run, monkeypatch, company_path, vertical_path):
        def interrupt(*_a, **_k):
            raise KeyboardInterrupt

        monkeypatch.setattr(cli, "load_vertical", interrupt)
        assert run("validate", "--config", str(company_path), "--vertical", str(vertical_path))[0] == 130

    def test_no_command_is_usage_error(self, capsys):
        with pytest.raises(SystemExit) as ei:
            cli.main([])
        assert ei.value.code == 2
