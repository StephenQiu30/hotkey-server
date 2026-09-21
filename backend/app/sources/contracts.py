from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Annotated, Literal, Protocol

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

_TOKEN_PATTERN = r"^[A-Za-z0-9._~-]+$"
_MAX_TEXT_LENGTH = 100_000


class SourceCapability(StrEnum):
    SEARCH = "search"
    AUTHOR_POSTS = "author_posts"
    COMMENTS = "comments"
    REPLIES = "replies"


class SourcePageState(StrEnum):
    MORE = "more"
    COMPLETE = "complete"
    EMPTY = "empty"
    PARTIAL = "partial"
    STOPPED = "stopped"


class SourceStopReason(StrEnum):
    END_OF_RESULTS = "end_of_results"
    SOURCE_EMPTY = "source_empty"
    RATE_LIMITED = "rate_limited"
    AUTHENTICATION_REQUIRED = "authentication_required"
    ACCESS_DENIED = "access_denied"
    NOT_FOUND = "not_found"
    UNSUPPORTED = "unsupported"
    CANCELLED = "cancelled"
    BUDGET_EXHAUSTED = "budget_exhausted"
    UPSTREAM_ERROR = "upstream_error"
    PROTOCOL_ERROR = "protocol_error"


class _ContractModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)


def _validate_identifier(value: str | None) -> str | None:
    if value is None:
        return None
    if value != value.strip() or any(
        ord(character) < 32 or ord(character) == 127 for character in value
    ):
        raise ValueError("source identifiers cannot contain surrounding whitespace or controls")
    return value


class _PageRequest(_ContractModel):
    source_key: str = Field(
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    page_size: int = Field(ge=1, le=100)
    page_token: str | None = Field(
        default=None,
        min_length=1,
        max_length=2048,
        pattern=_TOKEN_PATTERN,
    )
    watermark: str | None = Field(
        default=None,
        min_length=1,
        max_length=2048,
        pattern=_TOKEN_PATTERN,
    )


class SearchRequest(_PageRequest):
    capability: Literal[SourceCapability.SEARCH] = SourceCapability.SEARCH
    query: str = Field(min_length=1, max_length=512)

    @field_validator("query")
    @classmethod
    def validate_query(cls, value: str) -> str:
        if value != value.strip() or any(
            ord(character) < 32 or ord(character) == 127 for character in value
        ):
            raise ValueError("query cannot contain surrounding whitespace or controls")
        return value


class AuthorPostsRequest(_PageRequest):
    capability: Literal[SourceCapability.AUTHOR_POSTS] = SourceCapability.AUTHOR_POSTS
    author_external_id: str = Field(min_length=1, max_length=512)

    @field_validator("author_external_id")
    @classmethod
    def validate_author_external_id(cls, value: str) -> str:
        validated = _validate_identifier(value)
        assert validated is not None
        return validated


class CommentsRequest(_PageRequest):
    capability: Literal[SourceCapability.COMMENTS] = SourceCapability.COMMENTS
    post_external_id: str = Field(min_length=1, max_length=512)

    @field_validator("post_external_id")
    @classmethod
    def validate_post_external_id(cls, value: str) -> str:
        validated = _validate_identifier(value)
        assert validated is not None
        return validated


class RepliesRequest(_PageRequest):
    capability: Literal[SourceCapability.REPLIES] = SourceCapability.REPLIES
    comment_external_id: str = Field(min_length=1, max_length=512)

    @field_validator("comment_external_id")
    @classmethod
    def validate_comment_external_id(cls, value: str) -> str:
        validated = _validate_identifier(value)
        assert validated is not None
        return validated


type SourceRequest = Annotated[
    SearchRequest | AuthorPostsRequest | CommentsRequest | RepliesRequest,
    Field(discriminator="capability"),
]


class SourcePost(_ContractModel):
    object_type: Literal["post"] = "post"
    source_key: str = Field(
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    external_id: str = Field(min_length=1, max_length=512)
    author_external_id: str | None = Field(min_length=1, max_length=512)
    published_at: datetime | None
    text: str | None = Field(max_length=_MAX_TEXT_LENGTH)
    like_count: int | None = Field(ge=0)
    comment_count: int | None = Field(ge=0)
    repost_count: int | None = Field(ge=0)

    @field_validator("external_id", "author_external_id")
    @classmethod
    def validate_identifiers(cls, value: str | None) -> str | None:
        return _validate_identifier(value)

    @field_validator("published_at")
    @classmethod
    def validate_published_at(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.tzinfo is None:
            raise ValueError("published_at must be timezone-aware")
        return value


class SourceComment(_ContractModel):
    object_type: Literal["comment"] = "comment"
    source_key: str = Field(
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    external_id: str = Field(min_length=1, max_length=512)
    post_external_id: str = Field(min_length=1, max_length=512)
    parent_comment_external_id: str | None = Field(min_length=1, max_length=512)
    author_external_id: str | None = Field(min_length=1, max_length=512)
    published_at: datetime | None
    text: str | None = Field(max_length=_MAX_TEXT_LENGTH)
    like_count: int | None = Field(ge=0)

    @field_validator(
        "external_id",
        "post_external_id",
        "parent_comment_external_id",
        "author_external_id",
    )
    @classmethod
    def validate_identifiers(cls, value: str | None) -> str | None:
        return _validate_identifier(value)

    @field_validator("published_at")
    @classmethod
    def validate_published_at(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.tzinfo is None:
            raise ValueError("published_at must be timezone-aware")
        return value


type SourceItem = Annotated[SourcePost | SourceComment, Field(discriminator="object_type")]


class SourcePage(_ContractModel):
    source_key: str = Field(
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    capability: SourceCapability
    state: SourcePageState
    items: tuple[SourceItem, ...] = Field(max_length=100)
    next_page_token: str | None = Field(
        min_length=1,
        max_length=2048,
        pattern=_TOKEN_PATTERN,
    )
    watermark: str | None = Field(
        min_length=1,
        max_length=2048,
        pattern=_TOKEN_PATTERN,
    )
    stop_reason: SourceStopReason | None
    observed_at: datetime

    @field_validator("observed_at")
    @classmethod
    def validate_observed_at(cls, value: datetime) -> datetime:
        if value.tzinfo is None:
            raise ValueError("observed_at must be timezone-aware")
        return value

    @model_validator(mode="after")
    def validate_state(self) -> SourcePage:
        partial_reasons = {
            SourceStopReason.RATE_LIMITED,
            SourceStopReason.BUDGET_EXHAUSTED,
            SourceStopReason.UPSTREAM_ERROR,
            SourceStopReason.PROTOCOL_ERROR,
        }
        stopped_reasons = partial_reasons | {
            SourceStopReason.AUTHENTICATION_REQUIRED,
            SourceStopReason.ACCESS_DENIED,
            SourceStopReason.NOT_FOUND,
            SourceStopReason.UNSUPPORTED,
            SourceStopReason.CANCELLED,
        }
        expected_type = (
            SourcePost
            if self.capability in {SourceCapability.SEARCH, SourceCapability.AUTHOR_POSTS}
            else SourceComment
        )
        if any(
            item.source_key != self.source_key or not isinstance(item, expected_type)
            for item in self.items
        ):
            raise ValueError("source page items do not match its source and capability")
        if self.state is SourcePageState.MORE:
            valid = self.next_page_token is not None and self.stop_reason is None
        elif self.state is SourcePageState.COMPLETE:
            valid = (
                bool(self.items)
                and self.next_page_token is None
                and self.stop_reason is SourceStopReason.END_OF_RESULTS
            )
        elif self.state is SourcePageState.EMPTY:
            valid = (
                not self.items
                and self.next_page_token is None
                and self.stop_reason is SourceStopReason.SOURCE_EMPTY
            )
        elif self.state is SourcePageState.PARTIAL:
            valid = bool(self.items) and self.stop_reason in partial_reasons
        else:
            valid = (
                not self.items
                and self.next_page_token is None
                and self.stop_reason in stopped_reasons
            )
        if not valid:
            raise ValueError("source page fields do not match its state")
        return self


class SourceAdapter(Protocol):
    @property
    def source_key(self) -> str: ...

    @property
    def capabilities(self) -> frozenset[SourceCapability]: ...

    def fetch_page(self, request: SourceRequest) -> SourcePage: ...
