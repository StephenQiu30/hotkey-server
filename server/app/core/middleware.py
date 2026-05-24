from __future__ import annotations

import logging
import threading
import time
from collections import defaultdict
from collections import deque
from collections.abc import Callable
from typing import Any

from fastapi import status
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import JSONResponse

from server.app.core.settings import settings

logger = logging.getLogger("ai_hotspot_radar")


def mask_sensitive_headers(raw_headers: list[tuple[bytes, bytes]]) -> dict[str, str]:
    sensitive = {"authorization", "cookie", "x-api-key", "api-key"}
    result: dict[str, str] = {}
    for key, value in raw_headers:
        name = key.decode("latin1").lower()
        if name in sensitive:
            result[name] = "***"
        else:
            result[name] = value.decode("latin1")
    return result


def _clean_path(path: str) -> str:
    return path.replace("<", "").replace(">", "")


class RequestAuditMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request, call_next: Callable):
        start = time.perf_counter()
        response = await call_next(request)
        elapsed_ms = int((time.perf_counter() - start) * 1000)
        _record_request_metrics(response.status_code)
        audit_enabled = getattr(settings, "app_env", "").lower() != "test"
        if audit_enabled:
            safe_headers = mask_sensitive_headers(list(request.headers.raw))
            logger.info(
                "http_request",
                extra={
                    "method": request.method,
                    "path": _clean_path(request.url.path),
                    "query": request.url.query,
                    "status": response.status_code,
                    "elapsed_ms": elapsed_ms,
                    "headers": safe_headers,
                    "client": request.client.host if request.client else "unknown",
                },
            )
        return response


class _RateBucket:
    def __init__(self, limit: int) -> None:
        self.limit = limit
        self.timestamps: deque[float] = deque()


class RateLimitMiddleware(BaseHTTPMiddleware):
    def __init__(self, app, requests_per_minute: int = 120) -> None:
        super().__init__(app)
        self.requests_per_minute = max(requests_per_minute, 1)
        self.window_seconds = 60
        self._buckets: dict[str, _RateBucket] = {}
        self._lock = threading.Lock()

    async def dispatch(self, request, call_next: Callable):
        if self.requests_per_minute <= 0:
            return await call_next(request)
        client = _extract_client_id(request)
        now = time.monotonic()
        with self._lock:
            bucket = self._buckets.setdefault(client, _RateBucket(self.requests_per_minute))
            while bucket.timestamps and now - bucket.timestamps[0] > self.window_seconds:
                bucket.timestamps.popleft()
            if len(bucket.timestamps) >= bucket.limit:
                logger.warning("rate_limit_exceeded", extra={"client": client, "path": request.url.path})
                _record_request_metrics(status.HTTP_429_TOO_MANY_REQUESTS)
                _record_rate_limit_blocked()
                _record_active_rate_limit_clients(self._count_active_clients(now))
                return JSONResponse(
                    status_code=429,
                    content={"error": {"code": "rate_limit", "message": "请求过于频繁，请稍后重试。"}},
                )
            bucket.timestamps.append(now)
            _record_active_rate_limit_clients(self._count_active_clients(now))
        response = await call_next(request)
        response.headers["X-RateLimit-Limit"] = str(self.requests_per_minute)
        response.headers["X-RateLimit-Remaining"] = str(max(self.requests_per_minute - len(bucket.timestamps), 0))
        return response

    def _count_active_clients(self, now: float) -> int:
        count = 0
        for bucket in self._buckets.values():
            while bucket.timestamps and now - bucket.timestamps[0] > self.window_seconds:
                bucket.timestamps.popleft()
            if bucket.timestamps:
                count += 1
        return count


def _extract_client_id(request: Any) -> str:
    """Extract a stable client identifier for rate limiting under reverse proxy."""
    forwarded = request.headers.get("x-forwarded-for")
    if forwarded:
        # Keep the most left IP as the original client IP.
        first = forwarded.split(",")[0].strip()
        if first:
            return first
    real_ip = request.headers.get("x-real-ip")
    if real_ip:
        return real_ip.strip() or "anonymous"
    if request.client and request.client.host:
        return request.client.host
    return "anonymous"


_metrics_lock = threading.Lock()
_request_metrics: dict[str, int] = {
    "total_requests": 0,
    "status_2xx": 0,
    "status_3xx": 0,
    "status_4xx": 0,
    "status_5xx": 0,
    "status_unknown": 0,
    "active_rate_limit_clients": 0,
    "rate_limit_exceeded_total": 0,
}
_status_buckets: dict[str, int] = defaultdict(int)


def _status_bucket_for_code(status_code: int) -> str:
    if 200 <= status_code < 300:
        return "2xx"
    if 300 <= status_code < 400:
        return "3xx"
    if 400 <= status_code < 500:
        return "4xx"
    if 500 <= status_code < 600:
        return "5xx"
    return "unknown"


def _record_request_metrics(status_code: int) -> None:
    with _metrics_lock:
        _request_metrics["total_requests"] += 1
        bucket = _status_bucket_for_code(status_code)
        _request_metrics[f"status_{bucket}"] += 1
        _status_buckets[str(status_code)] += 1


def _record_rate_limit_blocked() -> None:
    with _metrics_lock:
        _request_metrics["rate_limit_exceeded_total"] += 1


def _record_active_rate_limit_clients(active_clients: int) -> None:
    with _metrics_lock:
        _request_metrics["active_rate_limit_clients"] = active_clients


def reset_request_metrics() -> None:
    with _metrics_lock:
        _request_metrics["total_requests"] = 0
        _request_metrics["status_2xx"] = 0
        _request_metrics["status_3xx"] = 0
        _request_metrics["status_4xx"] = 0
        _request_metrics["status_5xx"] = 0
        _request_metrics["status_unknown"] = 0
        _request_metrics["active_rate_limit_clients"] = 0
        _status_buckets.clear()
        _request_metrics["rate_limit_exceeded_total"] = 0


def get_request_metrics() -> dict[str, Any]:
    with _metrics_lock:
        return {
            "total_requests": _request_metrics["total_requests"],
            "status_buckets": dict(_status_buckets),
            "status_by_class": {
                "2xx": _request_metrics["status_2xx"],
                "3xx": _request_metrics["status_3xx"],
                "4xx": _request_metrics["status_4xx"],
                "5xx": _request_metrics["status_5xx"],
                "unknown": _request_metrics["status_unknown"],
            },
            "active_rate_limit_clients": _request_metrics["active_rate_limit_clients"],
            "rate_limit_exceeded_total": _request_metrics["rate_limit_exceeded_total"],
        }
