from typing import Any

from core.schemas import ErrorView

READ_ERROR_CODES = (401, 413, 422, 500, 503)
WRITE_ERROR_CODES = (401, 403, 413, 422, 500, 503)
PUBLIC_WRITE_ERROR_CODES = (403, 413, 422, 500, 503)
ERROR_DESCRIPTIONS = {
    400: "Invalid request",
    401: "Authentication required",
    403: "Forbidden",
    404: "Resource not found",
    409: "State conflict",
    413: "Request content too large",
    422: "Validation failed",
    429: "Rate limited",
    500: "Internal error",
    503: "Service unavailable",
}


def error_responses(*status_codes: int) -> dict[int | str, dict[str, Any]]:
    """Build an operation-local OpenAPI error contract."""
    return {
        status_code: {
            "model": ErrorView,
            "description": ERROR_DESCRIPTIONS.get(status_code, "Request failed"),
        }
        for status_code in status_codes
    }
