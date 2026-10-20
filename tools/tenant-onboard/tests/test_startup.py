"""Start-up expense loader tests. All data is mock data (tests/fixtures/).

Mock loadable rows: line 2 $100.00 at 100%, line 3 $45.00 at 50%, line 4
$25.00 at 100%. Original total $170.00; company share $147.50.
"""

from __future__ import annotations

import copy
import re
from datetime import date
from decimal import Decimal

from pathlib import Path

import pytest
import yaml

from tenant_onboard import cli
from tenant_onboard.cli import main
from tenant_onboard.company import parse_company
from tenant_onboard.errors import InputFileError, RenderError
from tenant_onboard.startup import (
    comment_safe,
    Row,
    build_entries,
    classify,
    read_extract,
    reconcile,
    render_manifest,
    render_sql,
)

FIXTURES = Path(__file__).resolve().parent / "fixtures"
EXTRACT = FIXTURES / "startup_mock.csv"
PARTNERSHIP = FIXTURES / "company_partnership.yaml"


@pytest.fixture
def partnership():
    """Mock two-member partnership; member ``b`` has a mock name, ``a`` does not."""
    return parse_company(copy.deepcopy(yaml.safe_load(PARTNERSHIP.read_text())))


@pytest.fixture
def parsed():
    rows, rejections = read_extract(EXTRACT)
    return classify(rows, rejections)


def _row(**over) -> Row:
    base = dict(line=2, when=date(2026, 3, 20), vendor="Mock Vendor X", reference="MOCK-9",
                description="Mock", amount=Decimal("100.00"), status="candidate",
                attribution="100%", notes="")
    base.update(over)
    return Row(**base)


# ── parsing and schema checks ─────────────────────────────────────────────

def test_read_rejects_bad_rows_with_line_numbers():
    _, rejections = read_extract(EXTRACT)
    by_line = {r.line: r.reason for r in rejections}
    assert by_line == {
        11: "missing date",
        12: "missing amount_usd",
        13: "missing status",
        14: "unknown status 'weird-status'",
    }


def test_read_keeps_every_well_formed_row():
    rows, _ = read_extract(EXTRACT)
    assert [r.line for r in rows] == [2, 3, 4, 5, 6, 7, 8, 9, 10]
    assert rows[0].amount == Decimal("100.00")
    assert rows[4].amount == Decimal("-10.00")


def test_control_characters_in_text_fields_are_rejected(tmp_path):
    """A newline in vendor/reference would end a generated SQL comment early."""
    bad = tmp_path / "ctrl.csv"
    bad.write_text(
        "date,vendor,reference,description,amount_usd,status,attribution,source,notes\n"
        '2026-03-20,"Mock\nDROP TABLE x;",MOCK-1,Hosting,5.00,candidate,100%,JSON,\n'
        "2026-03-21,Mock Vendor,MOCK\t2,Hosting,5.00,candidate,100%,JSON,\n",
        encoding="utf-8",
    )
    rows, rejections = read_extract(bad)
    reasons = [r.reason for r in rejections]
    assert rows == []
    assert "vendor contains a control character" in reasons
    assert "reference contains a control character" in reasons


def test_comment_safe_strips_line_breaks():
    assert "\n" not in comment_safe("Mock\nDROP TABLE x;")
    assert "\r" not in comment_safe("a\rb")


def test_rendered_comments_stay_on_one_line(partnership):
    entries = build_entries(classify([_row(vendor="Mock\nDROP TABLE x;")]).loadable)
    text = render_sql(entries, partnership, "b")
    comment = next(l for l in text.splitlines() if l.startswith("-- line 2"))
    assert "DROP" in comment and "\n" not in comment


def test_missing_file_is_input_error(tmp_path):
    with pytest.raises(InputFileError, match="cannot read"):
        read_extract(tmp_path / "absent.csv")


def test_missing_column_is_input_error(tmp_path):
    bad = tmp_path / "bad.csv"
    bad.write_text("date,vendor,reference\n2026-03-20,Mock,MOCK-1\n", encoding="utf-8")
    with pytest.raises(InputFileError, match="missing column"):
        read_extract(bad)


def test_non_utf8_is_input_error(tmp_path):
    bad = tmp_path / "latin1.csv"
    bad.write_bytes("date,vendor\n2026-03-20,Caf\xe9\n".encode("latin-1"))
    with pytest.raises(InputFileError, match="not valid UTF-8"):
        read_extract(bad)


def test_empty_file_is_input_error(tmp_path):
    empty = tmp_path / "empty.csv"
    empty.write_text("", encoding="utf-8")
    with pytest.raises(InputFileError, match="no header"):
        read_extract(empty)


# ── status, hold and attribution rules ────────────────────────────────────

def test_only_candidate_rows_load(parsed):
    assert [i.row.line for i in parsed.loadable] == [2, 3, 4]


def test_excluded_and_pending_rows_are_skipped_with_reasons(parsed):
    reasons = {s.line: s.reason for s in parsed.skipped}
    assert reasons[5] == "status 'excluded-duplicate' is not loadable"
    assert reasons[6] == "status 'excluded-net-zero' is not loadable"
    assert reasons[7] == "status 'pending-unpaid' is not loadable"


def test_duplicate_and_refund_rows_never_load(parsed):
    loaded_lines = {i.row.line for i in parsed.loadable}
    assert 5 not in loaded_lines  # duplicate of line 2
    assert 6 not in loaded_lines  # refund (negative)


def test_secretary_of_state_row_is_held_even_when_candidate(parsed):
    reasons = {s.line: s.reason for s in parsed.skipped}
    assert reasons[8].startswith("held:")
    assert 8 not in {i.row.line for i in parsed.loadable}


def test_zero_percent_attribution_is_skipped(parsed):
    reasons = {s.line: s.reason for s in parsed.skipped}
    assert reasons[9] == "attribution 0%"


def test_tbd_row_blocks_and_is_listed(parsed):
    assert [r.line for r in parsed.tbd] == [10]
    assert 10 not in {i.row.line for i in parsed.loadable}


def test_fractional_share_keeps_original_amount(parsed):
    half = next(i for i in parsed.loadable if i.row.line == 3)
    assert half.row.amount == Decimal("45.00")
    assert half.fraction == Decimal("0.5")
    assert half.company_amount == Decimal("22.50")


def test_company_share_rounds_half_up_to_cents():
    plan = classify([_row(amount=Decimal("0.05"), attribution="50%")])
    assert plan.loadable[0].company_amount == Decimal("0.03")


def test_loadable_row_with_non_positive_amount_is_rejected():
    plan = classify([_row(amount=Decimal("-5.00"))])
    assert plan.loadable == ()
    assert "must be positive" in plan.rejections[0].reason


def test_attribution_above_100_is_rejected():
    plan = classify([_row(attribution="150%")])
    assert plan.loadable == ()
    assert "above 100%" in plan.rejections[0].reason


def test_non_percentage_attribution_is_rejected():
    plan = classify([_row(attribution="half")])
    assert "not a percentage" in plan.rejections[0].reason


# ── reconciliation ────────────────────────────────────────────────────────

def test_reconciliation_counts_tbd_rows_as_candidates(parsed):
    # Candidate total = loadable $170.00 + TBD $20.00 (line 10).
    recon = reconcile(parsed.loadable, parsed.tbd, Decimal("190.00"))
    assert recon.candidate_total == Decimal("190.00")
    assert recon.loadable_total == Decimal("170.00")
    assert recon.company_total == Decimal("147.50")
    assert recon.matches


def test_reconciliation_reports_difference_and_never_forces(parsed):
    recon = reconcile(parsed.loadable, parsed.tbd, Decimal("1752.57"))
    assert not recon.matches
    assert recon.difference == Decimal("-1562.57")


# ── entries and SQL ───────────────────────────────────────────────────────

def test_duplicate_entry_key_is_an_error():
    plan = classify([_row(line=2), _row(line=3)])  # same date and reference
    with pytest.raises(RenderError, match="already used by line 2"):
        build_entries(plan.loadable)


def test_reference_without_key_characters_is_an_error():
    plan = classify([_row(reference="---")])
    with pytest.raises(RenderError, match="no entry key"):
        build_entries(plan.loadable)


def test_entries_keep_original_and_company_share(parsed):
    entries = build_entries(parsed.loadable)
    numbers = [e.entry_number for e in entries]
    assert numbers == ["SU-20260320-MOCK0001", "SU-20260321-MOCK0002", "SU-20260322-MOCK0003"]
    assert "Original $45.00 at 50% company share $22.50" in entries[1].narration
    assert "not capital" in entries[1].narration


def test_sql_is_balanced_usd_draft_and_idempotent(partnership, parsed):
    entries = build_entries(parsed.loadable)
    text = render_sql(entries, partnership, "b")
    assert text.count("BEGIN;") == 1 and text.count("COMMIT;") == 1
    assert text.count("ON CONFLICT (entry_number) DO NOTHING") == len(entries)
    assert "'USD'" in text and "INR" not in text
    assert text.count("'draft'") == len(entries)
    # Each entry: debit 1310 and credit 2300 carry the same company amount.
    debits = re.findall(r"SELECT je\.id, \(SELECT id FROM accounts[^\n]*'1310'\), ([\d.]+), 0,", text)
    credits = re.findall(r"SELECT je\.id, \(SELECT id FROM accounts[^\n]*'2300'\), 0, ([\d.]+),", text)
    assert debits == credits == ["100.00", "22.50", "25.00"]


def test_sql_credit_is_tagged_to_owed_member_party(partnership, parsed):
    text = render_sql(build_entries(parsed.loadable), partnership, "b")
    party = re.findall(r"VALUES \('([0-9a-f-]{36})', '[0-9a-f-]{36}', 'Robin", text)
    assert party, "party row for member b not rendered"
    assert text.count(f"'{party[0]}'::uuid FROM je") == 3  # every credit line, typed as uuid


def test_sql_escapes_member_name(partnership, parsed):
    text = render_sql(build_entries(parsed.loadable), partnership, "b")
    assert "O''Hara" in text


def test_party_id_is_deterministic(partnership, parsed):
    entries = build_entries(parsed.loadable)
    assert render_sql(entries, partnership, "b") == render_sql(entries, partnership, "b")


def test_owed_member_without_name_is_an_error(partnership, parsed):
    with pytest.raises(RenderError, match="has no name"):
        render_sql(build_entries(parsed.loadable), partnership, "a")


def test_unknown_owed_member_is_an_error(partnership, parsed):
    with pytest.raises(RenderError, match="no member 'z'"):
        render_sql(build_entries(parsed.loadable), partnership, "z")


def test_manifest_has_original_and_company_columns(parsed):
    lines = render_manifest(build_entries(parsed.loadable)).splitlines()
    assert lines[0] == "line,date,vendor,reference,entry_number,amount_usd,company_pct,company_usd"
    assert lines[2] == "3,2026-03-21,Mock Vendor B,MOCK-0002,SU-20260321-MOCK0002,45.00,50,22.50"


# ── CLI gates ─────────────────────────────────────────────────────────────

def _cli(*extra: str) -> int:
    return main(["startup-load", "--config", str(PARTNERSHIP), "--owed-to", "b", *extra])


def test_cli_blocks_on_tbd_and_writes_nothing(tmp_path, capsys):
    out = tmp_path / "out"
    code = _cli("--extract", str(EXTRACT), "--expect-total", "190.00", "--write", "--out", str(out))
    assert code == 2
    assert not out.exists()
    err = capsys.readouterr().err
    assert "attribution TBD (lines 10)" in err


def test_cli_dry_run_reports_mismatch_and_writes_nothing(tmp_path, capsys):
    code = _cli("--extract", str(EXTRACT), "--expect-total", "1752.57")
    assert code == 2
    assert "expected by $-1562.57" in capsys.readouterr().err


def test_cli_clean_extract_dry_run_then_write(tmp_path, capsys):
    clean = tmp_path / "clean.csv"
    lines = EXTRACT.read_text(encoding="utf-8").splitlines()
    keep = [lines[0], lines[1], lines[2], lines[3]]  # header + the three loadable rows
    clean.write_text("\n".join(keep) + "\n", encoding="utf-8")

    assert _cli("--extract", str(clean), "--expect-total", "170.00") == 0
    assert "READY: dry run" in capsys.readouterr().out

    out = tmp_path / "out"
    assert _cli("--extract", str(clean), "--expect-total", "170.00", "--write", "--out", str(out)) == 0
    assert (out / cli.STARTUP_SQL).exists()
    assert (out / cli.STARTUP_MANIFEST).exists()


def test_cli_write_requires_out(tmp_path, capsys):
    clean = tmp_path / "clean.csv"
    lines = EXTRACT.read_text(encoding="utf-8").splitlines()
    clean.write_text("\n".join(lines[:4]) + "\n", encoding="utf-8")
    assert _cli("--extract", str(clean), "--expect-total", "170.00", "--write") == 2
    assert "requires --out" in capsys.readouterr().err
