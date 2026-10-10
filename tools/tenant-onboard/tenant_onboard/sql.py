"""Safe SQL literal rendering.

The generator emits plain ``.sql`` files that are applied with ``psql``, so
values cannot be bound as query parameters. Every value that reaches SQL text
therefore goes through one of these helpers, which either produce a correctly
escaped literal or raise :class:`~tenant_onboard.errors.RenderError`.

Escaping assumes ``standard_conforming_strings = on`` (the PostgreSQL default
since 9.1), under which doubling single quotes is the complete escaping rule
for a standard string literal.
"""

from __future__ import annotations

import json
import uuid
from datetime import date
from typing import Any

from .errors import RenderError


def text(value: str | None) -> str:
    """Render a ``TEXT`` literal, or ``NULL`` for ``None``.

    Args:
        value: The string to quote.

    Returns:
        A single-quoted SQL literal such as ``'O''Brien'``, or ``NULL``.

    Raises:
        RenderError: If ``value`` is not a string or contains a NUL byte,
            which PostgreSQL text cannot store.
    """
    if value is None:
        return "NULL"
    if not isinstance(value, str):
        raise RenderError(f"expected str for SQL text literal, got {type(value).__name__}")
    if "\x00" in value:
        raise RenderError("NUL byte is not allowed in SQL text literal")
    return "'" + value.replace("'", "''") + "'"


def uuid_lit(value: str | uuid.UUID) -> str:
    """Render a UUID literal after validating it.

    Args:
        value: A UUID object or its canonical string form.

    Returns:
        A quoted, lower-case canonical UUID literal.

    Raises:
        RenderError: If ``value`` is not a valid UUID.
    """
    try:
        canonical = str(value if isinstance(value, uuid.UUID) else uuid.UUID(str(value)))
    except (ValueError, AttributeError, TypeError) as exc:
        raise RenderError(f"invalid UUID {value!r}") from exc
    return f"'{canonical}'"


def boolean(value: bool) -> str:
    """Render a ``BOOLEAN`` literal.

    Raises:
        RenderError: If ``value`` is not a ``bool`` (so ``1`` or ``"yes"``
            cannot slip through as truthy).
    """
    if not isinstance(value, bool):
        raise RenderError(f"expected bool, got {type(value).__name__}")
    return "true" if value else "false"


def integer(value: int) -> str:
    """Render an integer literal.

    Raises:
        RenderError: If ``value`` is not an ``int`` (``bool`` is rejected).
    """
    if isinstance(value, bool) or not isinstance(value, int):
        raise RenderError(f"expected int, got {type(value).__name__}")
    return str(value)


def date_lit(value: date | None) -> str:
    """Render a ``DATE`` literal in ISO form, or ``NULL``.

    Raises:
        RenderError: If ``value`` is not a :class:`datetime.date`.
    """
    if value is None:
        return "NULL"
    if not isinstance(value, date):
        raise RenderError(f"expected date, got {type(value).__name__}")
    return f"'{value.isoformat()}'"


def jsonb(value: Any, indent: int | None = None) -> str:
    """Render a ``JSONB`` literal from a JSON-serialisable Python value.

    Keys keep their insertion order so the output is stable and diffable.

    Args:
        value: The value to encode.
        indent: If given, pretty-print with this indent (for reviewable
            migrations); otherwise emit compact JSON.

    Raises:
        RenderError: If ``value`` cannot be serialised to JSON.
    """
    try:
        separators = (",", ": ") if indent is not None else (",", ":")
        encoded = json.dumps(value, ensure_ascii=False, indent=indent, separators=separators, allow_nan=False)
    except (TypeError, ValueError) as exc:
        raise RenderError(f"value is not JSON-serialisable: {exc}") from exc
    return text(encoded) + "::jsonb"


def text_array(values: list[str]) -> str:
    """Render a ``TEXT[]`` literal using the ``ARRAY[...]`` constructor.

    Raises:
        RenderError: If any element is not a safe string.
    """
    return "ARRAY[" + ",".join(text(v) for v in values) + "]::text[]"
