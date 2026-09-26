from __future__ import annotations

import json
from enum import StrEnum
from typing import Any, Self
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


class Sentiment(StrEnum):
    POSITIVE = "positive"
    NEUTRAL = "neutral"
    NEGATIVE = "negative"


class AnnotationStatus(StrEnum):
    ANNOTATED = "annotated"
    UNANALYZED = "unanalyzed"


class WindowAnnotationCountView(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    total_count: int = Field(ge=0)
    annotated_count: int = Field(ge=0)
    pending_count: int = Field(ge=0)
    failed_count: int = Field(ge=0)
    abnormal_count: int = Field(ge=0)


class AnalysisPromptItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    content_id: UUID
    content_version_id: UUID
    title: str | None = Field(default=None, max_length=2_000)
    body: str | None = Field(default=None, max_length=100_000)
    comments: tuple[str, ...] = Field(default=(), max_length=50)

    @model_validator(mode="after")
    def require_text(self) -> Self:
        if self.title is None and self.body is None:
            raise ValueError("analysis content requires title or body text")
        if any(not comment for comment in self.comments):
            raise ValueError("analysis comments must not be empty")
        return self


class AnalysisJobScope(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    topic_id: UUID
    topic_rule_version: int = Field(ge=1)
    prompt_version: str = Field(min_length=1, max_length=128)
    content_version_ids: tuple[UUID, ...] = Field(min_length=1, max_length=30)

    @field_validator("content_version_ids")
    @classmethod
    def require_distinct_versions(cls, value: tuple[UUID, ...]) -> tuple[UUID, ...]:
        if len(set(value)) != len(value):
            raise ValueError("analysis content versions must be distinct")
        return value

    @classmethod
    def from_job_scope(cls, scope: dict[str, str | int | bool | None]) -> AnalysisJobScope:
        encoded_ids = scope.get("content_version_ids")
        if not isinstance(encoded_ids, str):
            raise ValueError("analysis scope content_version_ids must be JSON")
        try:
            content_version_ids = json.loads(encoded_ids)
        except json.JSONDecodeError as error:
            raise ValueError("analysis scope content_version_ids must be JSON") from error
        return cls.model_validate(
            {
                "topic_id": scope.get("topic_id"),
                "topic_rule_version": scope.get("topic_rule_version"),
                "prompt_version": scope.get("prompt_version"),
                "content_version_ids": content_version_ids,
            }
        )


class AnnotationOutputEnvelope(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    items: list[Any] = Field(max_length=30)


class AnnotationOutputItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    content_version_id: UUID
    relevant: bool
    relevance_reason: str = Field(min_length=1, max_length=500)
    sentiment: Sentiment | None
    summary: str = Field(min_length=1, max_length=60)
    viewpoints: tuple[str, ...] = Field(max_length=5)

    @field_validator("relevant", mode="before")
    @classmethod
    def require_json_boolean(cls, value: object) -> object:
        if not isinstance(value, bool):
            raise ValueError("relevant must be a boolean")
        return value

    @field_validator("relevance_reason", "summary")
    @classmethod
    def reject_blank_text(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("annotation text must not be blank")
        return value

    @field_validator("viewpoints")
    @classmethod
    def validate_viewpoints(cls, value: tuple[str, ...]) -> tuple[str, ...]:
        if any(not item.strip() or len(item) > 200 for item in value):
            raise ValueError("viewpoints must be short non-empty sentences")
        return value

    @model_validator(mode="after")
    def validate_sentiment_relevance(self) -> Self:
        if self.relevant and self.sentiment is None:
            raise ValueError("relevant content requires sentiment")
        if not self.relevant and self.sentiment is not None:
            raise ValueError("irrelevant content must not declare sentiment")
        return self


class AnnotationWrite(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    content_version_id: UUID
    relevant: bool | None = None
    relevance_reason: str | None = None
    sentiment: Sentiment | None = None
    summary: str | None = None
    viewpoints: tuple[str, ...] = Field(default=(), max_length=5)
    ai_call_id: UUID | None = None
    status: AnnotationStatus

    @model_validator(mode="after")
    def validate_status_fields(self) -> Self:
        if self.status is AnnotationStatus.ANNOTATED and (
            self.relevant is None
            or self.relevance_reason is None
            or self.summary is None
            or (self.relevant and self.sentiment is None)
            or (not self.relevant and self.sentiment is not None)
        ):
            raise ValueError("annotated rows require consistent structured output")
        if self.status is AnnotationStatus.UNANALYZED and (
            self.relevant is not None
            or self.relevance_reason is not None
            or self.sentiment is not None
            or self.summary is not None
            or self.viewpoints
        ):
            raise ValueError("unanalyzed rows cannot contain inferred output")
        return self
