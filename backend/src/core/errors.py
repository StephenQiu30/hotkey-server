class ApplicationError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.message = message


class DependencyUnavailableError(ApplicationError):
    """A required runtime dependency cannot currently serve requests."""
