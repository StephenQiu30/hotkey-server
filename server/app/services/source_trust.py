from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from urllib.parse import parse_qsl, urlparse
from typing import Any

from server.app.core.settings import settings
from server.app.models.hotspot import Hotspot


@dataclass(slots=True)
class SourceEvidence:
    source_reachable: bool
    url_stability: bool
    domain_risk: float
    publish_depth: float
    cross_source_count: int
    status: str
    risk_tags: list[str]

    def truth_score(self) -> float:
        source_reachable_score = 100.0 if self.source_reachable else 0.0
        return round(
            0.35 * source_reachable_score
            + 0.20 * (100.0 if self.url_stability else 0.0)
            + 0.20 * self.domain_risk
            + 0.15 * self.publish_depth
            + 0.10 * min(100.0, float(self.cross_source_count) * 20.0),
            2,
        )

    def risk_level(self) -> str:
        score = self.truth_score()
        if score >= 80:
            return "high"
        if score >= 60:
            return "medium"
        return "low"

    def penalty(self) -> float:
        if self.risk_level() == "low":
            return float(settings.low_trust_penalty)
        return 0.0

    def bundle(self) -> dict[str, Any]:
        return {
            "version": 1,
            "source_reachable": self.source_reachable,
            "url_stability": self.url_stability,
            "domain_risk": self.domain_risk,
            "publish_depth": self.publish_depth,
            "cross_source_count": self.cross_source_count,
            "status": self.status,
            "risk_tags": self.risk_tags,
        }


def _domain(hostname: str | None) -> str:
    if not hostname:
        return ""
    return hostname.lower().lstrip(".")


def _domain_risk_for_host(hostname: str) -> tuple[float, list[str]]:
    low_risk_domains = {
        "twitter.com",
        "x.com",
        "github.com",
        "news.ycombinator.com",
        "www.hndigest.com",
        "bing.com",
    }
    high_risk_domains = {"bit.ly", "t.co", "tinyurl.com", "ow.ly"}
    if hostname in low_risk_domains:
        return 90.0, []
    if hostname in high_risk_domains:
        return 40.0, ["shortlink"]
    if hostname.endswith(".com") or hostname.endswith(".cn") or hostname.endswith(".org"):
        return 70.0, []
    return 65.0, ["domain_unknown"]


def collect_source_evidence(hotspot: Hotspot, *, cross_source_count: int = 1) -> SourceEvidence:
    """Collect low-cost, deterministic evidence signals for source trust scoring."""
    now = datetime.now(timezone.utc)
    parsed = urlparse((hotspot.url or "").strip())
    hostname = _domain(parsed.hostname)
    params = {k.lower() for k, _ in parse_qsl(parsed.query or "", keep_blank_values=True)}
    url_stability = not any(tag in {"utm_source", "utm_medium", "fbclid", "yclid", "ref"} for tag in params)
    source_reachable = bool(parsed.scheme in {"http", "https"} and hostname)

    domain_risk_score, domain_tags = _domain_risk_for_host(hostname)
    tags = list(dict.fromkeys(domain_tags))
    if not url_stability:
        tags.append("query_noise")
    if parsed.fragment:
        tags.append("fragment")

    publish_depth = 100.0 if hotspot.published_at and hotspot.published_at <= now else 60.0

    status = "ok" if source_reachable else "unreachable"
    return SourceEvidence(
        source_reachable=source_reachable,
        url_stability=url_stability,
        domain_risk=domain_risk_score,
        publish_depth=publish_depth,
        cross_source_count=max(1, cross_source_count),
        status=status,
        risk_tags=tags,
    )
