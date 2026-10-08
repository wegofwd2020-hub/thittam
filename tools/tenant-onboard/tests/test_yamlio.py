"""Tests for tenant_onboard.yamlio — every way a YAML input can fail."""

import pytest

from tenant_onboard.errors import InputFileError
from tenant_onboard.yamlio import load_yaml_mapping


def test_loads_mapping(write_yaml):
    assert load_yaml_mapping(write_yaml("ok.yaml", {"a": 1})) == {"a": 1}


def test_missing_file(tmp_path):
    with pytest.raises(InputFileError, match="file not found"):
        load_yaml_mapping(tmp_path / "nope.yaml")


def test_directory(tmp_path):
    with pytest.raises(InputFileError, match="is a directory"):
        load_yaml_mapping(tmp_path)


def test_invalid_yaml_reports_line(write_yaml):
    p = write_yaml("bad.yaml", "a: 1\nb: [unclosed\n")
    with pytest.raises(InputFileError, match=r"invalid YAML at line \d+"):
        load_yaml_mapping(p)


def test_empty_file(write_yaml):
    with pytest.raises(InputFileError, match="empty"):
        load_yaml_mapping(write_yaml("empty.yaml", ""))


def test_top_level_list_rejected(write_yaml):
    with pytest.raises(InputFileError, match="must be a mapping, got list"):
        load_yaml_mapping(write_yaml("list.yaml", "- a\n- b\n"))


def test_non_utf8(tmp_path):
    p = tmp_path / "latin1.yaml"
    p.write_bytes("name: caf\xe9\n".encode("latin-1"))
    with pytest.raises(InputFileError, match="UTF-8"):
        load_yaml_mapping(p)


def test_error_carries_path(tmp_path):
    with pytest.raises(InputFileError) as ei:
        load_yaml_mapping(tmp_path / "x.yaml")
    assert ei.value.path.endswith("x.yaml")
