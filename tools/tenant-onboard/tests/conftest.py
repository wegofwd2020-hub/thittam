"""Shared fixtures. All data is mock data from tests/fixtures/."""

from __future__ import annotations

import copy
import sys
from pathlib import Path
from typing import Any, Callable

import pytest
import yaml

TOOL_DIR = Path(__file__).resolve().parents[1]
FIXTURES = Path(__file__).resolve().parent / "fixtures"
REPO_ROOT = TOOL_DIR.parents[1]

sys.path.insert(0, str(TOOL_DIR))

from tenant_onboard.company import parse_company  # noqa: E402
from tenant_onboard.vertical import load_vertical  # noqa: E402


@pytest.fixture
def fixtures_dir() -> Path:
    return FIXTURES


@pytest.fixture
def vertical_path() -> Path:
    return FIXTURES / "vertical_minimal.yaml"


@pytest.fixture
def vertical(vertical_path):
    return load_vertical(vertical_path)


@pytest.fixture
def company_path() -> Path:
    return FIXTURES / "company_valid.yaml"


@pytest.fixture
def company_dict(company_path) -> dict[str, Any]:
    """A fresh, mutable copy of the valid mock company mapping."""
    return yaml.safe_load(company_path.read_text())


@pytest.fixture
def company(company_dict):
    return parse_company(copy.deepcopy(company_dict))


@pytest.fixture
def vertical_dict(vertical_path) -> dict[str, Any]:
    return yaml.safe_load(vertical_path.read_text())


@pytest.fixture
def write_yaml(tmp_path) -> Callable[[str, Any], Path]:
    """Write a mapping (or raw text) to tmp_path/name and return the path."""

    def _write(name: str, data: Any) -> Path:
        p = tmp_path / name
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(data if isinstance(data, str) else yaml.safe_dump(data, sort_keys=False))
        return p

    return _write
