"""Exception hierarchy for tenant_onboard.

Every error the tool raises on purpose derives from :class:`OnboardError`, so
callers (and the CLI) can catch one type and still tell user mistakes apart
from bugs. Errors that describe *input* problems carry a list of
:class:`Issue` objects so a single run reports every problem at once instead
of failing on the first one.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Issue:
    """One validation problem, located by a dotted field path.

    Attributes:
        field: Dotted path to the offending field, e.g. ``"address.city"``.
        message: Human-readable description of what is wrong.
    """

    field: str
    message: str

    def __str__(self) -> str:
        return f"{self.field}: {self.message}"


class OnboardError(Exception):
    """Base class for every intentional error raised by tenant_onboard."""


class InputFileError(OnboardError):
    """A required input file is missing, unreadable, or not valid YAML."""

    def __init__(self, path: str, reason: str) -> None:
        self.path = path
        self.reason = reason
        super().__init__(f"{path}: {reason}")


class ValidationError(OnboardError):
    """Input parsed correctly but failed one or more validation rules.

    Attributes:
        source: What was being validated (a file path or a logical name).
        issues: Every problem found, in discovery order. Never empty.
    """

    def __init__(self, source: str, issues: list[Issue]) -> None:
        if not issues:
            raise ValueError("ValidationError requires at least one issue")
        self.source = source
        self.issues = list(issues)
        lines = "\n".join(f"  - {i}" for i in self.issues)
        super().__init__(f"{source}: {len(self.issues)} validation issue(s)\n{lines}")


class CompanyConfigError(ValidationError):
    """The company (tenant) configuration is invalid."""


class VerticalConfigError(ValidationError):
    """The vertical YAML is invalid for onboarding purposes."""


class RenderError(OnboardError):
    """SQL rendering failed (e.g. a value cannot be safely quoted)."""


class OutputError(OnboardError):
    """Generated files could not be written."""

    def __init__(self, path: str, reason: str) -> None:
        self.path = path
        self.reason = reason
        super().__init__(f"cannot write {path}: {reason}")
