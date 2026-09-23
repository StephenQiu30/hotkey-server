from __future__ import annotations

import json
import re
from collections.abc import Callable
from dataclasses import dataclass
from urllib.parse import urlsplit

import httpx
from pydantic import SecretStr

from sources.contracts import SourceStopReason

_HANDLE = re.compile(r"[A-Za-z0-9_]{1,15}\Z", re.ASCII)
_USER_ID = re.compile(r"[0-9]{1,19}\Z", re.ASCII)
_RESERVED = frozenset(
    {"explore", "home", "i", "intent", "messages", "notifications", "search", "settings"}
)
_MAX_RESPONSE_BYTES = 128 * 1024


def parse_x_handle(value: str) -> str:
    """Extract a handle without fetching or following a profile URL."""
    if not isinstance(value, str) or value != value.strip():
        raise ValueError("invalid X profile input")
    if value.startswith("https://"):
        parsed = urlsplit(value)
        if (
            parsed.scheme != "https"
            or parsed.hostname not in {"x.com", "www.x.com"}
            or parsed.username is not None
            or parsed.password is not None
            or parsed.port is not None
            or parsed.query
            or parsed.fragment
        ):
            raise ValueError("invalid X profile URL")
        handle = parsed.path.removeprefix("/").removesuffix("/")
    else:
        handle = value.removeprefix("@")
    if _HANDLE.fullmatch(handle) is None or handle.casefold() in _RESERVED:
        raise ValueError("invalid X handle")
    return handle


@dataclass(frozen=True, slots=True)
class XUserCandidate:
    source_key: str
    external_id: str
    username: str
    display_name: str


@dataclass(frozen=True, slots=True)
class XUserLookupResult:
    user: XUserCandidate | None
    stop_reason: SourceStopReason | None
    request_count: int


class XUserLookupAdapter:
    """Offline-only official User Lookup. The caller owns paid budget settlement."""

    def __init__(
        self,
        *,
        token: SecretStr,
        transport: httpx.MockTransport,
        authorize_request: Callable[[int, int], bool],
        settle_request: Callable[[int, int | None], None],
    ) -> None:
        raw_token = token.get_secret_value()
        if not raw_token or any(ord(char) < 33 or ord(char) > 126 for char in raw_token):
            raise ValueError("invalid developer token")
        if not isinstance(transport, httpx.MockTransport):
            raise ValueError("the offline adapter requires a mock transport")
        self._token = token
        self._transport = transport
        self._authorize_request = authorize_request
        self._settle_request = settle_request

    def lookup(self, profile_input: str) -> XUserLookupResult:
        handle = parse_x_handle(profile_input)
        if not self._authorize_request(1, 1):
            return XUserLookupResult(None, SourceStopReason.BUDGET_EXHAUSTED, 0)

        returned_users: int | None = None
        try:
            with (
                httpx.Client(
                    transport=self._transport,
                    follow_redirects=False,
                    timeout=10.0,
                ) as client,
                client.stream(
                    "GET",
                    f"https://api.x.com/2/users/by/username/{handle}",
                    headers={"Authorization": f"Bearer {self._token.get_secret_value()}"},
                ) as response,
            ):
                reason = {
                    401: SourceStopReason.AUTHENTICATION_REQUIRED,
                    403: SourceStopReason.ACCESS_DENIED,
                    404: SourceStopReason.NOT_FOUND,
                    429: SourceStopReason.RATE_LIMITED,
                }.get(response.status_code)
                if reason is not None:
                    return XUserLookupResult(None, reason, 1)
                if response.status_code >= 500:
                    return XUserLookupResult(None, SourceStopReason.UPSTREAM_ERROR, 1)
                if response.status_code != 200:
                    return XUserLookupResult(None, SourceStopReason.PROTOCOL_ERROR, 1)
                content = bytearray()
                for chunk in response.iter_bytes():
                    content.extend(chunk)
                    if len(content) > _MAX_RESPONSE_BYTES:
                        return XUserLookupResult(None, SourceStopReason.PROTOCOL_ERROR, 1)
            try:
                payload = json.loads(content)
            except (UnicodeDecodeError, json.JSONDecodeError):
                return XUserLookupResult(None, SourceStopReason.PROTOCOL_ERROR, 1)
            if (
                not isinstance(payload, dict)
                or payload.get("errors")
                or not isinstance(payload.get("data"), dict)
            ):
                return XUserLookupResult(None, SourceStopReason.PROTOCOL_ERROR, 1)
            user = payload["data"]
            identifier, username, name = user.get("id"), user.get("username"), user.get("name")
            if (
                not isinstance(identifier, str)
                or _USER_ID.fullmatch(identifier) is None
                or not isinstance(username, str)
                or _HANDLE.fullmatch(username) is None
                or username.casefold() != handle.casefold()
                or not isinstance(name, str)
                or not 1 <= len(name) <= 50
                or any(ord(character) < 32 or ord(character) == 127 for character in name)
            ):
                return XUserLookupResult(None, SourceStopReason.PROTOCOL_ERROR, 1)
            returned_users = 1
            return XUserLookupResult(XUserCandidate("x", identifier, username, name), None, 1)
        except (httpx.TimeoutException, httpx.TransportError):
            return XUserLookupResult(None, SourceStopReason.UPSTREAM_ERROR, 1)
        finally:
            self._settle_request(1, returned_users)
