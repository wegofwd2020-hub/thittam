"""YAML file loading with errors that name the file and the cause."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml

from .errors import InputFileError


def load_yaml_mapping(path: str | Path) -> dict[str, Any]:
    """Load a YAML file whose top level must be a mapping.

    Args:
        path: File to read.

    Returns:
        The parsed top-level mapping.

    Raises:
        InputFileError: If the file is missing, unreadable, not UTF-8, not
            valid YAML, or its top level is not a mapping.
    """
    p = Path(path)
    try:
        raw = p.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise InputFileError(str(p), "file not found") from exc
    except IsADirectoryError as exc:
        raise InputFileError(str(p), "is a directory, expected a YAML file") from exc
    except PermissionError as exc:
        raise InputFileError(str(p), "permission denied") from exc
    except UnicodeDecodeError as exc:
        raise InputFileError(str(p), f"not valid UTF-8 ({exc.reason})") from exc
    except OSError as exc:
        raise InputFileError(str(p), f"cannot read file ({exc.strerror or exc})") from exc

    try:
        data = yaml.safe_load(raw)
    except yaml.YAMLError as exc:
        mark = getattr(exc, "problem_mark", None)
        where = f" at line {mark.line + 1}, column {mark.column + 1}" if mark else ""
        raise InputFileError(str(p), f"invalid YAML{where}") from exc

    if data is None:
        raise InputFileError(str(p), "file is empty")
    if not isinstance(data, dict):
        raise InputFileError(str(p), f"top level must be a mapping, got {type(data).__name__}")
    return data
