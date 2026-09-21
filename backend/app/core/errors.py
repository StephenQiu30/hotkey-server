from __future__ import annotations

from collections.abc import Mapping
from enum import StrEnum
from types import MappingProxyType

type ErrorContextValue = str | int | float | bool | None


class ErrorCategory(StrEnum):
    AUTHENTICATION = "authentication"
    AUTHORIZATION = "authorization"
    CONFLICT = "conflict"
    DEPENDENCY_UNAVAILABLE = "dependency_unavailable"
    INVALID_INPUT = "invalid_input"
    NOT_FOUND = "not_found"


ERROR_CATEGORIES: Mapping[str, ErrorCategory] = MappingProxyType(
    {
        "bootstrap_forbidden": ErrorCategory.AUTHORIZATION,
        "csrf_invalid": ErrorCategory.AUTHORIZATION,
        "database_unavailable": ErrorCategory.DEPENDENCY_UNAVAILABLE,
        "identity_already_initialized": ErrorCategory.CONFLICT,
        "identity_not_initialized": ErrorCategory.NOT_FOUND,
        "idempotency_conflict": ErrorCategory.CONFLICT,
        "invalid_credentials": ErrorCategory.AUTHENTICATION,
        "invalid_password": ErrorCategory.INVALID_INPUT,
        "invalid_session": ErrorCategory.AUTHENTICATION,
        "job_not_cancellable": ErrorCategory.CONFLICT,
        "resource_not_found": ErrorCategory.NOT_FOUND,
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
