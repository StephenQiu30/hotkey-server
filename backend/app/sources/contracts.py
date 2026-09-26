from __future__ import annotations

from datetime import datetime, timedelta
from enum import StrEnum
from typing import Annotated, Literal, Protocol
from urllib.parse import urlsplit

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

_TOKEN_PATTERN = r"^[A-Za-z0-9._~-]+$"
_MAX_TEXT_LENGTH = 100_000


class SourceCapability(StrEnum):
    SEARCH = "search"
    AUTHOR_POSTS = "author_posts"
    COMMENTS = "comments"
    REPLIES = "replies"
    PAGE_CONTENT = "page_content"
    HOTLIST = "hotlist"


type SocialSourceCapability = Literal[
    SourceCapability.SEARCH,
    SourceCapability.AUTHOR_POSTS,
    SourceCapability.COMMENTS,
    SourceCapability.REPLIES,
]


SOCIAL_CAPABILITIES: tuple[SocialSourceCapability, ...] = (
    SourceCapability.SEARCH,
    SourceCapability.AUTHOR_POSTS,
    SourceCapability.COMMENTS,
    SourceCapability.REPLIES,
)


class SourceSort(StrEnum):
    LATEST = "latest"
    TOP = "top"


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
    CURSOR_EXPIRED = "cursor_expired"
    CURSOR_LOOP = "cursor_loop"


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
    sort: SourceSort = SourceSort.LATEST
    starts_at: datetime | None = None
    ends_at: datetime | None = None

    @field_validator("query")
    @classmethod
    def validate_query(cls, value: str) -> str:
        if value != value.strip() or any(
            ord(character) < 32 or ord(character) == 127 for character in value
        ):
            raise ValueError("query cannot contain surrounding whitespace or controls")
        return value

    @field_validator("starts_at", "ends_at")
    @classmethod
    def validate_window_bound(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.utcoffset() != timedelta(0):
            raise ValueError("search window bounds must be UTC")
        return value

    @model_validator(mode="after")
    def validate_window(self) -> SearchRequest:
        if (self.starts_at is None) != (self.ends_at is None):
            raise ValueError("search window requires both bounds")
        if (
            self.starts_at is not None
            and self.ends_at is not None
            and self.starts_at >= self.ends_at
        ):
            raise ValueError("search window end must follow start")
        return self


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
    post_external_id: str | None = Field(default=None, min_length=1, max_length=512)

    @field_validator("comment_external_id", "post_external_id")
    @classmethod
    def validate_comment_external_id(cls, value: str | None) -> str | None:
        return _validate_identifier(value)


type SourceRequest = Annotated[
    SearchRequest | AuthorPostsRequest | CommentsRequest | RepliesRequest,
    Field(discriminator="capability"),
]


class WebPageRequest(_ContractModel):
    capability: Literal[SourceCapability.PAGE_CONTENT] = SourceCapability.PAGE_CONTENT
    url: str = Field(min_length=1, max_length=2048)
    timeout_seconds: int = Field(default=20, ge=1, le=20)
    max_content_characters: int = Field(default=100_000, ge=1, le=100_000)
    refresh: Literal["fresh"] = "fresh"

    @field_validator("url")
    @classmethod
    def validate_url(cls, value: str) -> str:
        if value != value.strip() or any(
            ord(character) < 32 or ord(character) == 127 for character in value
        ):
            raise ValueError("web page URL cannot contain whitespace or controls")
        try:
            parsed = urlsplit(value)
            port = parsed.port
        except ValueError as error:
            raise ValueError("web page URL is invalid") from error
        scheme = parsed.scheme.lower()
        if (
            scheme not in {"http", "https"}
            or parsed.hostname is None
            or parsed.username is not None
            or parsed.password is not None
            or port not in {None, 80 if scheme == "http" else 443}
        ):
            raise ValueError("web page URL is not an allowed HTTP target")
        return value


class SourceDocument(_ContractModel):
    request_url: str = Field(min_length=1, max_length=2048)
    final_url: str = Field(min_length=1, max_length=2048)
    title: str | None = Field(default=None, max_length=512)
    text: str = Field(min_length=1, max_length=_MAX_TEXT_LENGTH)
    text_scope: Literal["full", "truncated"]
    observed_at: datetime
    published_at: datetime | None
    content_fingerprint: str = Field(pattern=r"^[0-9a-f]{64}$")
    extractor_version: str = Field(min_length=1, max_length=64)

    @field_validator("observed_at", "published_at")
    @classmethod
    def validate_datetimes(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.utcoffset() is None:
            raise ValueError("document timestamps must be timezone-aware")
        return value


class WebPageResult(_ContractModel):
    document: SourceDocument | None = None
    stop_reason: SourceStopReason | None = None
    target_status_code: int | None = Field(default=None, ge=100, le=599)
    collector_call_count: int = Field(ge=0, le=1)
    target_request_count: int | None = Field(default=None, ge=0)

    @model_validator(mode="after")
    def validate_result(self) -> WebPageResult:
        if self.document is not None:
            valid = (
                self.stop_reason is None
                and self.target_status_code is not None
                and 200 <= self.target_status_code < 300
                and self.collector_call_count == 1
            )
        else:
            valid = self.stop_reason is not None and (
                self.collector_call_count == 1
                or (self.target_status_code is None and self.target_request_count is None)
            )
        if not valid:
            raise ValueError("web page result fields do not match its outcome")
        return self


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
    language: str | None = Field(default=None, min_length=2, max_length=35)
    like_count: int | None = Field(ge=0)
    comment_count: int | None = Field(ge=0)
    repost_count: int | None = Field(ge=0)
    canonical_url: str | None = Field(default=None, max_length=2048)
    conversation_external_id: str | None = Field(default=None, min_length=1, max_length=512)
    parent_external_id: str | None = Field(default=None, min_length=1, max_length=512)
    quote_external_id: str | None = Field(default=None, min_length=1, max_length=512)
    repost_external_id: str | None = Field(default=None, min_length=1, max_length=512)
    text_scope: Literal["full", "truncated", "media_only"] | None = None
    title: str | None = Field(default=None, min_length=1, max_length=2000)
    author_name: str | None = Field(default=None, min_length=1, max_length=256)
    view_count: int | None = Field(default=None, ge=0)
    play_count: int | None = Field(default=None, ge=0)
    danmaku_count: int | None = Field(default=None, ge=0)

    @field_validator(
        "external_id",
        "author_external_id",
        "conversation_external_id",
        "parent_external_id",
        "quote_external_id",
        "repost_external_id",
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
    author_name: str | None = Field(default=None, min_length=1, max_length=256)
    canonical_url: str | None = Field(default=None, max_length=2048)

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
    capability: SocialSourceCapability
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
    request_count: int = Field(default=0, ge=0)
    adapter_version: str | None = Field(default=None, max_length=64)
    retry_at: datetime | None = None

    @field_validator("retry_at")
    @classmethod
    def validate_retry_at(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.utcoffset() is None:
            raise ValueError("retry_at must be timezone-aware")
        return value

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
            SourceStopReason.CURSOR_EXPIRED,
            SourceStopReason.CURSOR_LOOP,
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
    def capabilities(self) -> frozenset[SocialSourceCapability]: ...

    def fetch_page(self, request: SourceRequest) -> SourcePage: ...


class HotlistEntry(_ContractModel):
    rank: int = Field(ge=1, le=100)
    title: str = Field(min_length=1, max_length=2000)
    url: str = Field(min_length=1, max_length=2048)
    summary: str | None = Field(default=None, max_length=100_000)
    published_at: datetime | None = None
    heat: str | None = Field(default=None, max_length=256)

    @field_validator("published_at")
    @classmethod
    def validate_published_at(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.tzinfo is None:
            raise ValueError("published_at must be timezone-aware")
        return value

    @field_validator("url")
    @classmethod
    def validate_url(cls, value: str) -> str:
        parsed = urlsplit(value)
        if (
            parsed.scheme not in {"http", "https"}
            or not parsed.hostname
            or parsed.username
            or parsed.password
        ):
            raise ValueError("hotlist entry URL must be public HTTP(S)")
        return value


class HotlistPage(_ContractModel):
    source_key: str = Field(pattern=r"^[a-z][a-z0-9_-]{0,63}$", max_length=64)
    capability: Literal[SourceCapability.HOTLIST] = SourceCapability.HOTLIST
    state: SourcePageState
    items: tuple[HotlistEntry, ...] = Field(max_length=100)
    stop_reason: SourceStopReason | None = None
    observed_at: datetime
    request_count: int = Field(ge=0, le=1)
    adapter_version: str = Field(pattern=r"^[A-Za-z0-9][A-Za-z0-9._+:-]{0,63}$")
    retry_at: datetime | None = None

    @model_validator(mode="after")
    def validate_page(self) -> HotlistPage:
        if self.observed_at.tzinfo is None or (
            self.retry_at is not None and self.retry_at.tzinfo is None
        ):
            raise ValueError("hotlist page times must be timezone-aware")
        if self.state is SourcePageState.COMPLETE:
            valid = bool(self.items) and self.stop_reason is SourceStopReason.END_OF_RESULTS
        elif self.state is SourcePageState.EMPTY:
            valid = not self.items and self.stop_reason is SourceStopReason.SOURCE_EMPTY
        elif self.state is SourcePageState.STOPPED:
            valid = not self.items and self.stop_reason not in {
                None,
                SourceStopReason.END_OF_RESULTS,
                SourceStopReason.SOURCE_EMPTY,
            }
        else:
            valid = False
        if not valid or any(item.rank != index for index, item in enumerate(self.items, 1)):
            raise ValueError("invalid hotlist page state or ranks")
        return self


class DocumentAdapter(Protocol):
    def fetch_document(self, request: WebPageRequest) -> WebPageResult: ...
