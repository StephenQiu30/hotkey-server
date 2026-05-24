from __future__ import annotations

from datetime import datetime, timezone, timedelta

from server.app.core.settings import settings
from server.app.models.source import Source

_MAX_HEALTH_SCORE = 100.0


def _to_float(value: object, *, default: float = 100.0) -> float:
    if isinstance(value, int | float):
        return float(value)
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def _ensure_health_fields(source: Source) -> dict[str, object]:
    """Ensure source.config and health buckets exist so selector logic is side-effect safe."""
    if isinstance(source.config, dict):
        payload = source.config
    else:
        payload = {}
        source.config = payload

    health = payload.get("health")
    if not isinstance(health, dict):
        health = {}
        payload["health"] = health
    return health


def _health_score(source: Source, now: datetime) -> tuple[str, float, bool]:
    health = _ensure_health_fields(source)
    status = str(health.get("status", "healthy") or "healthy").strip().lower() or "healthy"
    consecutive_failures = int(health.get("consecutive_failures", 0) or 0)
    last_error_time = health.get("last_error_at")
    if status == "degraded":
        if isinstance(last_error_time, str):
            try:
                parsed = datetime.fromisoformat(last_error_time.replace("Z", "+00:00"))
                if (now - parsed) > timedelta(seconds=settings.source_health_window_seconds):
                    status = "recovering"
                    consecutive_failures = max(0, consecutive_failures - 1)
            except ValueError:
                status = "degraded"
        else:
            status = "degraded"
    base_score = max(0.0, _MAX_HEALTH_SCORE - consecutive_failures * 10.0)
    return status, base_score, status == "degraded"


def select_sources(sources: list[Source]) -> list[Source]:
    now = datetime.now(timezone.utc)

    status_rank = {
        "healthy": 0,
        "recovering": 1,
        "degraded": 2,
        "offline": 3,
    }

    def _sort_key(source: Source) -> tuple[int, float, int]:
        status, health_score, _ = _health_score(source, now)
        configured_weight = _to_float(source.config.get("weight", 0), default=0.0)
        return (status_rank.get(status, 3), -health_score, -configured_weight, source.id)

    return sorted(sources, key=_sort_key)


def mark_source_success(source: Source) -> None:
    """Record a successful fetch and refresh source health counters."""
    health = _ensure_health_fields(source)
    health["status"] = "healthy"
    health["consecutive_failures"] = 0
    health["success_count"] = int(health.get("success_count", 0) or 0) + 1
    health["last_success_at"] = datetime.now(timezone.utc).isoformat()


def mark_source_failure(source: Source, reason: str | None = None) -> None:
    health = _ensure_health_fields(source)
    health["failure_count"] = int(health.get("failure_count", 0) or 0) + 1
    health["consecutive_failures"] = int(health.get("consecutive_failures", 0) or 0) + 1
    health["last_error_at"] = datetime.now(timezone.utc).isoformat()
    if reason:
        health["last_error_reason"] = reason
    if int(health["consecutive_failures"]) >= settings.source_failure_threshold:
        health["status"] = "degraded"
    elif int(health.get("failure_count", 0)) > 10:
        health["status"] = "offline"
    else:
        health["status"] = "recovering"
