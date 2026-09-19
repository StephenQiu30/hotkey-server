class ApplicationError(Exception):
    def __init__(self, code: str, message: str, *, status_code: int = 400) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.status_code = status_code


class DependencyUnavailableError(ApplicationError):
    """A required runtime dependency cannot currently serve requests."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(code, message, status_code=503)
