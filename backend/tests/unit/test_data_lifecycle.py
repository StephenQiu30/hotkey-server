from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest
from pydantic import ValidationError

from evidence.schemas import (
    AccessBasis,
    AccessPolicyStatus,
    SourceAccessPolicyInput,
    SourceCapability,
)
from evidence.services import minimize_payload


def _approved_input(**changes: object) -> SourceAccessPolicyInput:
    reviewed_at = datetime(2026, 9, 21, 9, tzinfo=UTC)
    values: dict[str, object] = {
        "source_key": "douyin",
        "capability": SourceCapability.SEARCH,
        "status": AccessPolicyStatus.APPROVED,
        "enabled": True,
        "access_basis": AccessBasis.OFFICIAL_API,
        "terms_reference": "https://open.douyin.com/platform/resource/docs",
        "processing_purpose": "发现与已配置主题相关的公开作品",
        "component_name": "hotkey.sources.douyin",
        "component_version": "1.0.0",
        "component_license": "project-internal",
        "field_purposes": {
            "external_id": "稳定识别作品",
            "text": "核对主题相关性",
        },
        "reviewed_at": reviewed_at,
        "review_expires_at": reviewed_at + timedelta(days=30),
    }
    values.update(changes)
    return SourceAccessPolicyInput.model_validate(values)


def test_only_complete_approved_policy_can_be_enabled() -> None:
    with pytest.raises(ValidationError):
        _approved_input(status=AccessPolicyStatus.PENDING)

    with pytest.raises(ValidationError):
        _approved_input(terms_reference=None)

    with pytest.raises(ValidationError):
        _approved_input(field_purposes={})


def test_policy_rejects_secret_fields_and_invalid_review_window() -> None:
    with pytest.raises(ValidationError):
        _approved_input(field_purposes={"access_token": "调用来源接口"})

    reviewed_at = datetime(2026, 9, 21, 9, tzinfo=UTC)
    with pytest.raises(ValidationError):
        _approved_input(
            reviewed_at=reviewed_at,
            review_expires_at=reviewed_at,
        )


def test_minimize_payload_keeps_only_declared_top_level_fields() -> None:
    minimized = minimize_payload(
        field_purposes={
            "external_id": "稳定识别作品",
            "text": "核对主题相关性",
        },
        payload={
            "external_id": "video-1",
            "text": "公开正文",
            "author_profile": {"email": "not-needed@example.com"},
            "access_token": "must-not-survive",
        },
    )

    assert minimized == {"external_id": "video-1", "text": "公开正文"}

    with pytest.raises(ValueError, match="JSON scalars or scalar lists"):
        minimize_payload(
            field_purposes={"author_profile": "核对公开作者标识"},
            payload={"author_profile": {"id": "author-1"}},
        )
