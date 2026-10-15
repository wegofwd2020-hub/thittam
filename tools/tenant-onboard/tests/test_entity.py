"""Tests for tenant_onboard.entity — the entity block and owner-equity accounts.

All data is mock data: tests/fixtures/company_partnership.yaml and the small
mappings built inline below.
"""

from __future__ import annotations

import copy
from decimal import Decimal

import pytest
import yaml

from tenant_onboard.company import check_against_vertical, parse_company
from tenant_onboard.entity import (
    ENTITY_REQUIRED_VERTICALS,
    PARTNERSHIP,
    SCHEDULE_C,
    Entity,
    Member,
    check_entity_against_vertical,
    equity_accounts,
    parse_entity,
)
from tenant_onboard.errors import CompanyConfigError, Issue
from tenant_onboard.vertical import AccountEntry


def _parse(raw):
    issues: list[Issue] = []
    entity = parse_entity(raw, issues)
    return entity, issues


def _fields(issues):
    return [i.field for i in issues]


def _partnership(**members):
    return {"tax_classification": PARTNERSHIP, "members": members}


@pytest.fixture
def partnership_dict(fixtures_dir):
    return yaml.safe_load((fixtures_dir / "company_partnership.yaml").read_text())


@pytest.fixture
def partnership(partnership_dict):
    return parse_company(copy.deepcopy(partnership_dict))


class TestParseValid:
    def test_absent_block_means_no_entity(self):
        entity, issues = _parse(None)
        assert entity is None and issues == []

    def test_partnership_from_fixture(self, partnership):
        e = partnership.entity
        assert e.tax_classification == PARTNERSHIP and e.is_partnership
        assert [m.key for m in e.members] == ["a", "b"]
        assert [m.ownership_pct for m in e.members] == [Decimal(60), Decimal(40)]

    def test_name_whitespace_collapsed_and_used_for_display(self, partnership):
        a, b = partnership.entity.members
        assert (a.name, a.display) == (None, "Member A")
        assert (b.name, b.display) == ("Robin O'Hara", "Robin O'Hara")

    def test_single_owner(self):
        entity, issues = _parse({"tax_classification": SCHEDULE_C,
                                 "members": {"owner": {"label": "Owner", "ownership_pct": 100}}})
        assert issues == [] and not entity.is_partnership and len(entity.members) == 1

    def test_fractional_shares_are_exact(self):
        third = {"label": "X", "ownership_pct": "33.3333"}
        entity, issues = _parse(_partnership(a=third, b=dict(third), c={"label": "Y", "ownership_pct": "33.3334"}))
        assert issues == []
        assert sum(m.ownership_pct for m in entity.members) == Decimal(100)

    def test_float_shares_avoid_binary_noise(self):
        entity, issues = _parse(_partnership(a={"label": "A", "ownership_pct": 33.3},
                                             b={"label": "B", "ownership_pct": 66.7}))
        assert issues == []


class TestParseInvalid:
    def test_not_a_mapping(self):
        _, issues = _parse(["partnership"])
        assert _fields(issues) == ["entity"]

    def test_unknown_classification(self):
        _, issues = _parse({"tax_classification": "llp", "members": {"a": {"label": "A", "ownership_pct": 100}}})
        assert "entity.tax_classification" in _fields(issues)

    @pytest.mark.parametrize("tc", ["s_corp_1120s", "c_corp_1120"])
    def test_corporations_rejected_as_not_yet_supported(self, tc):
        _, issues = _parse({"tax_classification": tc, "members": {"a": {"label": "A", "ownership_pct": 100}}})
        assert any("not supported yet" in i.message for i in issues)

    def test_members_required(self):
        _, issues = _parse({"tax_classification": PARTNERSHIP})
        assert _fields(issues) == ["entity.members"]

    def test_members_must_be_mapping_not_list(self):
        _, issues = _parse({"tax_classification": PARTNERSHIP,
                            "members": [{"label": "A", "ownership_pct": 50}, {"label": "B", "ownership_pct": 50}]})
        assert "entity.members" in _fields(issues)

    def test_schedule_c_needs_exactly_one(self):
        _, issues = _parse({"tax_classification": SCHEDULE_C,
                            "members": {"a": {"label": "A", "ownership_pct": 50},
                                        "b": {"label": "B", "ownership_pct": 50}}})
        assert any("exactly one member" in i.message for i in issues)

    def test_partnership_needs_two(self):
        _, issues = _parse(_partnership(a={"label": "A", "ownership_pct": 100}))
        assert any("2 to 9 members" in i.message for i in issues)

    def test_partnership_at_most_nine(self):
        members = {f"m{i}": {"label": f"M{i}", "ownership_pct": 10} for i in range(10)}
        _, issues = _parse(_partnership(**members))
        assert any("2 to 9 members" in i.message for i in issues)

    def test_shares_must_total_100(self):
        _, issues = _parse(_partnership(a={"label": "A", "ownership_pct": 50},
                                        b={"label": "B", "ownership_pct": 40}))
        assert [i.message for i in issues] == ["ownership_pct must total 100, got 90"]

    @pytest.mark.parametrize("bad", [0, -5, 101, True, None, "half", [50], "1e400", "NaN"])
    def test_bad_share(self, bad):
        _, issues = _parse(_partnership(a={"label": "A", "ownership_pct": bad},
                                        b={"label": "B", "ownership_pct": 50}))
        assert "entity.members.a.ownership_pct" in _fields(issues)

    def test_too_many_decimal_places(self):
        _, issues = _parse(_partnership(a={"label": "A", "ownership_pct": "50.00001"},
                                        b={"label": "B", "ownership_pct": "49.99999"}))
        assert "entity.members.a.ownership_pct" in _fields(issues)

    @pytest.mark.parametrize("key", ["A", "1a", "a-b", ""])
    def test_bad_member_key(self, key):
        _, issues = _parse(_partnership(**{key: {"label": "A", "ownership_pct": 50}},
                                        b={"label": "B", "ownership_pct": 50}))
        assert f"entity.members.{key}" in _fields(issues)

    def test_member_must_be_mapping(self):
        _, issues = _parse(_partnership(a="Member A", b={"label": "B", "ownership_pct": 50}))
        assert "entity.members.a" in _fields(issues)

    def test_label_required(self):
        _, issues = _parse(_partnership(a={"ownership_pct": 50}, b={"label": "B", "ownership_pct": 50}))
        assert "entity.members.a.label" in _fields(issues)

    def test_name_must_be_text(self):
        _, issues = _parse(_partnership(a={"label": "A", "name": 7, "ownership_pct": 50},
                                        b={"label": "B", "ownership_pct": 50}))
        assert "entity.members.a.name" in _fields(issues)

    def test_name_length_capped(self):
        _, issues = _parse(_partnership(a={"label": "A", "name": "x" * 121, "ownership_pct": 50},
                                        b={"label": "B", "ownership_pct": 50}))
        assert "entity.members.a.name" in _fields(issues)

    def test_errors_reported_through_company(self, partnership_dict):
        partnership_dict["entity"]["members"]["b"]["ownership_pct"] = 30
        with pytest.raises(CompanyConfigError) as exc:
            parse_company(partnership_dict)
        assert "entity.members" in [i.field for i in exc.value.issues]


class TestEquityAccounts:
    def test_none_for_absent_entity(self):
        assert equity_accounts(None) == []

    def test_single_owner(self):
        e = Entity(SCHEDULE_C, (Member("owner", "Owner", None, Decimal(100)),))
        assert equity_accounts(e) == [
            AccountEntry("3100", "Owner's Contributions", "equity", "3000"),
            AccountEntry("3200", "Owner's Draws", "equity", "3000"),
        ]

    def test_partnership_codes_names_and_parents(self, partnership):
        rows = [(a.code, a.name, a.account_type, a.parent_code) for a in equity_accounts(partnership.entity)]
        assert rows == [
            ("3010", "Capital — Member A", "equity", "3000"),
            ("3020", "Capital — Robin O'Hara", "equity", "3000"),
            ("3110", "Distributions — Member A", "equity", "3000"),
            ("3120", "Distributions — Robin O'Hara", "equity", "3000"),
            ("3910", "Current-Year Earnings (clearing)", "equity", None),
            ("6050", "Guaranteed Payments to Partners", "expense", "6000"),
            ("6051", "Guaranteed Payments — Member A", "expense", "6050"),
            ("6052", "Guaranteed Payments — Robin O'Hara", "expense", "6050"),
        ]

    def test_parents_listed_before_children(self, partnership):
        seen = {"3000", "6000"}
        for a in equity_accounts(partnership.entity):
            assert a.parent_code is None or a.parent_code in seen
            seen.add(a.code)


class TestAgainstVertical:
    def test_fixture_partnership_fits_mock_vertical(self, partnership, vertical):
        check_against_vertical(partnership, vertical)

    def test_absent_entity_fine_for_other_verticals(self, vertical):
        assert check_entity_against_vertical(None, vertical.id, vertical.accounts) == []

    def test_absent_entity_rejected_where_required(self, vertical):
        issues = check_entity_against_vertical(None, ENTITY_REQUIRED_VERTICALS[0], vertical.accounts)
        assert _fields(issues) == ["entity"] and "required" in issues[0].message

    def test_code_clash_reported(self, partnership, vertical):
        clash = (*vertical.accounts, AccountEntry("3010", "Something Else", "equity", "3000"))
        issues = check_entity_against_vertical(partnership.entity, vertical.id, clash)
        assert any("clashes" in i.message and "3010" in i.message for i in issues)

    def test_missing_parent_reported(self, partnership, vertical):
        without_6000 = tuple(a for a in vertical.accounts if a.code not in ("6000", "6100"))
        issues = check_entity_against_vertical(partnership.entity, vertical.id, without_6000)
        assert any("no account 6000" in i.message for i in issues)

    def test_parent_type_mismatch_reported(self, partnership, vertical):
        wrong = tuple(AccountEntry(a.code, a.name, "liability", None) if a.code == "3000" else a
                      for a in vertical.accounts)
        issues = check_entity_against_vertical(partnership.entity, vertical.id, wrong)
        assert any("3000 is liability" in i.message for i in issues)
