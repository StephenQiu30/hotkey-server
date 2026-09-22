from __future__ import annotations

from ipaddress import ip_address
from urllib.parse import urlsplit, urlunsplit


def _canonical_host(value: str) -> str:
    try:
        return value.rstrip(".").encode("idna").decode("ascii").lower()
    except UnicodeError as error:
        raise ValueError("web target host is invalid") from error


def normalize_web_host(value: str) -> str:
    """Normalize one exact public domain used by a versioned web connection."""
    if (
        value != value.strip()
        or not value
        or any(ord(character) < 32 or ord(character) == 127 for character in value)
    ):
        raise ValueError("web target host is invalid")
    try:
        parsed = urlsplit(f"//{value}")
        port = parsed.port
    except ValueError as error:
        raise ValueError("web target host is invalid") from error
    if (
        parsed.hostname is None
        or parsed.username is not None
        or parsed.password is not None
        or port is not None
        or parsed.path
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("web target host is invalid")
    host = _canonical_host(parsed.hostname)
    try:
        ip_address(host)
    except ValueError:
        return host
    raise ValueError("web target host must be a public domain")


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
    host = normalize_web_host(parsed.hostname)
    allowed = frozenset(normalize_web_host(item) for item in allowed_hosts)
    if not allowed or host not in allowed:
        raise ValueError("web target host is not allowlisted")
    if port not in {None, 80 if scheme == "http" else 443}:
        raise ValueError("web target port is not allowed")

    path = parsed.path or "/"
    return urlunsplit((scheme, host, path, parsed.query, ""))
