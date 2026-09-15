from typing import Any

from core.schemas import ErrorView

READ_ERROR_CODES = (401, 413, 422, 500, 503)
WRITE_ERROR_CODES = (401, 403, 413, 422, 500, 503)
PUBLIC_WRITE_ERROR_CODES = (403, 413, 422, 500, 503)


def error_responses(*status_codes: int) -> dict[int | str, dict[str, Any]]:
    """Build an operation-local OpenAPI error contract."""
    return {status_code: {"model": ErrorView} for status_code in status_codes}
