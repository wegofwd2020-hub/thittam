"""Tests for tenant_onboard.company — parsing, overrides, periods, cross-checks."""

import copy
from datetime import date

import pytest

from tenant_onboard.company import (
    Books,
    check_against_vertical,
    deep_merge,
    load_company,
    parse_company,
)
from tenant_onboard.errors import CompanyConfigError, InputFileError


def _issue_fields(exc: CompanyConfigError) -> list[str]:
    return [i.field for i in exc.issues]


class TestValid:
    def test_core_fields(self, company):
        assert company.tenant_id == "0f6b1c2e-1111-4a5b-9c3d-000000000001"
        assert company.slug == "obrien-mock-studio"
        assert company.currency == "USD"
        assert company.address.line2 == "Suite 2"

    def test_name_whitespace_collapsed(self, company):
        assert company.name == "O'Brien Mock Studio LLC"

    def test_email_lowercased(self, company):
        assert company.admin.email == "owner@example.com"

    def test_products(self, company):
        assert [p.slug for p in company.products] == ["widget-app", "gadget-api"]
        assert company.products[0].start_date == date(2026, 10, 20)
        assert company.products[1].description is None

    def test_status_mapping(self, company):
        assert company.status_for("build") == "production"
        assert company.status_for("discovery") == "development"

    def test_fiscal_year_end_defaults_to_december(self, company_dict):
        del company_dict["books"]["fiscal_year_end_month"]
        assert parse_company(company_dict).books.fiscal_year_end_month == 12


class TestOverrides:
    def test_local_file_merged_automatically(self, fixtures_dir):
        c = load_company(fixtures_dir / "split" / "company.yaml")
        assert c.address.line1 == "7 Private Lane"
        assert c.address.postal_code == "48000-1234"
        assert c.admin.email == "mock.owner@example.test"
        assert c.plan == "professional"  # untouched base value survives

    def test_without_local_placeholders_are_reported(self, fixtures_dir, tmp_path):
        base = (fixtures_dir / "split" / "company.yaml").read_text()
        p = tmp_path / "company.yaml"
        p.write_text(base)
        with pytest.raises(CompanyConfigError) as ei:
            load_company(p)
        fields = _issue_fields(ei.value)
        for f in ("address.line1", "address.city", "address.postal_code", "admin.email"):
            assert f in fields
        assert "REPLACE_ME" in str(ei.value)

    def test_explicit_missing_override_is_an_error(self, company_path, tmp_path):
        with pytest.raises(InputFileError, match="file not found"):
            load_company(company_path, tmp_path / "missing.local.yaml")

    def test_deep_merge_does_not_mutate_inputs(self):
        base = {"a": {"b": 1, "c": [1]}, "d": 1}
        over = {"a": {"b": 2}, "e": 3}
        snapshot = copy.deepcopy(base)
        merged = deep_merge(base, over)
        assert merged == {"a": {"b": 2, "c": [1]}, "d": 1, "e": 3}
        assert base == snapshot

    def test_deep_merge_lists_replace(self):
        assert deep_merge({"x": [1, 2]}, {"x": [3]}) == {"x": [3]}


@pytest.mark.parametrize(
    ("path", "value", "field", "fragment"),
    [
        (("tenant", "id"), "not-a-uuid", "tenant.id", "UUID"),
        (("tenant", "slug"), "Bad Slug", "tenant.slug", "hyphens"),
        (("tenant", "slug"), "double--hyphen", "tenant.slug", "hyphens"),
        (("tenant", "plan"), "gold", "tenant.plan", "starter"),
        (("tenant", "primary_currency"), "usd", "tenant.primary_currency", "ISO 4217"),
        (("tenant", "name"), "x" * 201, "tenant.name", "200"),
        (("tenant", "name"), "   ", "tenant.name", "empty"),
        (("address", "country"), "USA", "address.country", "ISO 3166"),
        (("address", "postal_code"), "4999", "address.postal_code", "ZIP"),
        (("address", "postal_code"), 49999, "address.postal_code", "string"),
        (("address", "city"), None, "address.city", "required"),
        (("admin", "email"), "not-an-email", "admin.email", "email"),
        (("admin", "id"), "0f6b1c2e-1111-4a5b-9c3d-000000000001", "admin.id", "differ"),
        (("books", "opening_date"), "2026-10-20", "books.opening_date", "date"),
        (("books", "fiscal_year_end_month"), 13, "books.fiscal_year_end_month", "1-12"),
        (("books", "fiscal_year_end_month"), True, "books.fiscal_year_end_month", "1-12"),
    ],
)
def test_invalid_field(company_dict, path, value, field, fragment):
    section, key = path
    company_dict[section][key] = value
    with pytest.raises(CompanyConfigError) as ei:
        parse_company(company_dict)
    matching = [i for i in ei.value.issues if i.field == field]
    assert matching, _issue_fields(ei.value)
    assert fragment in matching[0].message


def test_non_us_postal_code_not_zip_checked(company_dict):
    company_dict["address"].update(country="CA", postal_code="K1A 0B1")
    assert parse_company(company_dict).address.postal_code == "K1A 0B1"


def test_missing_sections_all_reported():
    with pytest.raises(CompanyConfigError) as ei:
        parse_company({})
    fields = _issue_fields(ei.value)
    for f in ("tenant", "address", "admin", "books", "products"):
        assert f in fields


class TestProducts:
    def test_duplicate_slug(self, company_dict):
        company_dict["products"][1]["slug"] = "widget-app"
        with pytest.raises(CompanyConfigError, match="duplicate slug"):
            parse_company(company_dict)

    def test_end_before_start(self, company_dict):
        company_dict["products"][0]["end_date"] = date(2026, 1, 1)
        with pytest.raises(CompanyConfigError, match="before start_date"):
            parse_company(company_dict)

    def test_empty_list(self, company_dict):
        company_dict["products"] = []
        with pytest.raises(CompanyConfigError, match="non-empty"):
            parse_company(company_dict)

    def test_non_mapping_row(self, company_dict):
        company_dict["products"].append("oops")
        with pytest.raises(CompanyConfigError, match=r"products\[2\]: must be a mapping"):
            parse_company(company_dict)


class TestStageStatusMap:
    def test_override_applies(self, company_dict):
        company_dict["stage_status_map"] = {"build": "pre_production"}
        assert parse_company(company_dict).status_for("build") == "pre_production"

    def test_invalid_status_rejected(self, company_dict):
        company_dict["stage_status_map"] = {"build": "shipping"}
        with pytest.raises(CompanyConfigError, match="stage_status_map.build"):
            parse_company(company_dict)

    def test_must_be_mapping(self, company_dict):
        company_dict["stage_status_map"] = ["build"]
        with pytest.raises(CompanyConfigError, match="mapping"):
            parse_company(company_dict)


class TestOpenPeriods:
    def test_october_open_calendar_year(self):
        assert Books(date(2026, 10, 20), 12).open_periods() == [(2026, 10), (2026, 11), (2026, 12)]

    def test_crosses_year_for_june_fiscal_year(self):
        periods = Books(date(2026, 10, 20), 6).open_periods()
        assert periods[0] == (2026, 10) and periods[-1] == (2027, 6) and len(periods) == 9

    def test_opening_in_last_month(self):
        assert Books(date(2026, 12, 1), 12).open_periods() == [(2026, 12)]

    def test_opening_in_year_end_month_of_non_calendar_fy(self):
        assert Books(date(2026, 6, 15), 6).open_periods() == [(2026, 6)]


class TestCrossCheck:
    def test_ok(self, company, vertical):
        check_against_vertical(company, vertical)  # does not raise

    def test_vertical_mismatch(self, company_dict, vertical):
        company_dict["tenant"]["vertical"] = "construction"
        with pytest.raises(CompanyConfigError, match="tenant.vertical"):
            check_against_vertical(parse_company(company_dict), vertical)

    def test_unknown_stage(self, company_dict, vertical):
        company_dict["products"][0]["stage"] = "qa"
        with pytest.raises(CompanyConfigError, match="not a stage of test-studio"):
            check_against_vertical(parse_company(company_dict), vertical)

    def test_unmapped_stage(self, company_dict, vertical_dict):
        from tenant_onboard.vertical import parse_vertical

        vertical_dict["vertical"]["phase_types"].append({"id": "incubating", "label": "Incubating"})
        v = parse_vertical(vertical_dict)
        company_dict["products"][0]["stage"] = "incubating"
        with pytest.raises(CompanyConfigError, match="no productions.status mapping"):
            check_against_vertical(parse_company(company_dict), v)
