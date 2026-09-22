from __future__ import annotations

from ipaddress import ip_address
from urllib.parse import urlsplit, urlunsplit


def _canonical_host(value: str) -> str:
    try:
        return value.rstrip(".").encode("idna").decode("ascii").lower()
    except UnicodeError as error:
        raise ValueError("web target host is invalid") from error


def normalize_web_url(url: str, *, allowed_hosts: frozenset[str]) -> str:
    """Normalize an allowlisted public HTTP URL without changing path or query meaning."""
    try:
        parsed = urlsplit(url)
        port = parsed.port
    except ValueError as error:
        raise ValueError("web target URL is invalid") from error
    if (
        url != url.strip()
        or any(ord(character) < 32 or ord(character) == 127 for character in url)
        or parsed.scheme.lower() not in {"http", "https"}
        or parsed.hostname is None
        or parsed.username is not None
        or parsed.password is not None
    ):
        raise ValueError("web target URL is not allowed")

    scheme = parsed.scheme.lower()
    host = _canonical_host(parsed.hostname)
    allowed = frozenset(_canonical_host(item) for item in allowed_hosts)
    if not allowed or host not in allowed:
        raise ValueError("web target host is not allowlisted")
    try:
        address = ip_address(host)
    except ValueError:
        address = None
    if address is not None:
        raise ValueError("web target must use an allowlisted domain")
    if port not in {None, 80 if scheme == "http" else 443}:
        raise ValueError("web target port is not allowed")

    path = parsed.path or "/"
    return urlunsplit((scheme, host, path, parsed.query, ""))
