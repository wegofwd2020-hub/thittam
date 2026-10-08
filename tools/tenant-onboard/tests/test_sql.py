"""Tests for tenant_onboard.sql — literal rendering and its error paths."""

import json
import uuid
from datetime import date

import pytest

from tenant_onboard import sql
from tenant_onboard.errors import RenderError


class TestText:
    def test_plain(self):
        assert sql.text("abc") == "'abc'"

    def test_single_quotes_are_doubled(self):
        assert sql.text("O'Brien's") == "'O''Brien''s'"

    def test_backslash_is_literal_under_standard_conforming_strings(self):
        assert sql.text("a\\b") == "'a\\b'"

    def test_none_is_null(self):
        assert sql.text(None) == "NULL"

    def test_unicode_preserved(self):
        assert sql.text("திட்டம் — plan") == "'திட்டம் — plan'"

    def test_nul_byte_rejected(self):
        with pytest.raises(RenderError, match="NUL"):
            sql.text("a\x00b")

    @pytest.mark.parametrize("bad", [1, 1.5, b"x", ["x"]])
    def test_non_string_rejected(self, bad):
        with pytest.raises(RenderError, match="expected str"):
            sql.text(bad)


class TestUuid:
    def test_string_is_canonicalised(self):
        u = "0F6B1C2E-1111-4A5B-9C3D-000000000001"
        assert sql.uuid_lit(u) == "'0f6b1c2e-1111-4a5b-9c3d-000000000001'"

    def test_uuid_object(self):
        u = uuid.UUID(int=1)
        assert sql.uuid_lit(u) == f"'{u}'"

    @pytest.mark.parametrize("bad", ["", "not-a-uuid", "1234", None, "'; DROP TABLE x; --"])
    def test_invalid_rejected(self, bad):
        with pytest.raises(RenderError, match="invalid UUID"):
            sql.uuid_lit(bad)


class TestScalars:
    def test_boolean(self):
        assert sql.boolean(True) == "true" and sql.boolean(False) == "false"

    @pytest.mark.parametrize("bad", [1, 0, "true", None])
    def test_boolean_rejects_truthy_non_bool(self, bad):
        with pytest.raises(RenderError):
            sql.boolean(bad)

    def test_integer(self):
        assert sql.integer(2026) == "2026"

    @pytest.mark.parametrize("bad", [True, 1.0, "1", None])
    def test_integer_rejects_non_int(self, bad):
        with pytest.raises(RenderError):
            sql.integer(bad)

    def test_date(self):
        assert sql.date_lit(date(2026, 10, 20)) == "'2026-10-20'"
        assert sql.date_lit(None) == "NULL"

    def test_date_rejects_string(self):
        with pytest.raises(RenderError, match="expected date"):
            sql.date_lit("2026-10-20")


class TestJsonb:
    def test_round_trip_and_order(self):
        value = {"b": 1, "a": ["x", "O'k"], "ü": None}
        rendered = sql.jsonb(value)
        assert rendered.endswith("::jsonb")
        inner = rendered[1:-len("'::jsonb")].replace("''", "'")
        assert json.loads(inner) == value
        assert inner.index('"b"') < inner.index('"a"')

    @pytest.mark.parametrize("bad", [{"x": float("nan")}, {"x": object()}, {"x": {1, 2}}])
    def test_unserialisable_rejected(self, bad):
        with pytest.raises(RenderError, match="JSON"):
            sql.jsonb(bad)


def test_jsonb_pretty_is_multiline_and_equal():
    value = {"a": [1, {"b": "O'k"}]}
    rendered = sql.jsonb(value, indent=2)
    inner = rendered[1:-len("'::jsonb")].replace("''", "'")
    assert "\n" in inner and json.loads(inner) == value


def test_text_array():
    assert sql.text_array(["a", "b'c"]) == "ARRAY['a','b''c']::text[]"


def test_text_array_rejects_bad_element():
    with pytest.raises(RenderError):
        sql.text_array(["ok", 3])
