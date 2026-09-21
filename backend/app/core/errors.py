from __future__ import annotations

from collections.abc import Mapping
from enum import StrEnum
from types import MappingProxyType

type ErrorContextValue = str | int | float | bool | None


class ErrorCategory(StrEnum):
    DEPENDENCY_UNAVAILABLE = "dependency_unavailable"


ERROR_CATEGORIES: Mapping[str, ErrorCategory] = MappingProxyType(
    {
        "database_unavailable": ErrorCategory.DEPENDENCY_UNAVAILABLE,
    }
)


def get_error_category(code: str) -> ErrorCategory:
    try:
        return ERROR_CATEGORIES[code]
    except KeyError as error:
        raise ValueError(f"unregistered application error code: {code}") from error


class ApplicationError(Exception):
    def __init__(
        self,
        code: str,
        *,
        context: Mapping[str, ErrorContextValue] | None = None,
    ) -> None:
        get_error_category(code)
        super().__init__(code)
        self.code = code
        self.context = dict(context or {})


class DependencyUnavailableError(ApplicationError):
    """A required runtime dependency cannot currently serve requests."""

    def __init__(
        self,
        code: str = "database_unavailable",
        *,
        context: Mapping[str, ErrorContextValue] | None = None,
    ) -> None:
        if get_error_category(code) is not ErrorCategory.DEPENDENCY_UNAVAILABLE:
            raise ValueError(f"error code is not a dependency failure: {code}")
        super().__init__(code, context=context)
