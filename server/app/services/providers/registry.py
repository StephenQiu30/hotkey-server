from __future__ import annotations

from collections.abc import Callable
from typing import Any
from typing import TypeVar

from server.app.models.keyword import Keyword
from server.app.models.source import Source

from .base import BaseProvider, SourceIngestionError

ProviderClass = type[BaseProvider]
_ProviderDecorator = Callable[[ProviderClass], ProviderClass]
_ProviderClassT = TypeVar("_ProviderClassT", bound=ProviderClass)


def normalize_source_type(source_type: str) -> str:
    return source_type.strip().lower().replace("-", "_")


class ProviderRegistry:
    def __init__(self) -> None:
        self._providers: dict[str, ProviderClass] = {}

    def register(self, source_type: str, provider: ProviderClass, *aliases: str) -> None:
        keys = (source_type, *aliases)
        for key in keys:
            self._providers[normalize_source_type(key)] = provider

    def get(self, source_type: str) -> ProviderClass | None:
        return self._providers.get(normalize_source_type(source_type))

    def providers(self) -> dict[str, ProviderClass]:
        return dict(self._providers)


provider_registry = ProviderRegistry()


def register_provider(source_type: str, *aliases: str) -> Callable[[_ProviderClassT], _ProviderClassT]:
    def decorator(provider: _ProviderClassT) -> _ProviderClassT:
        provider_registry.register(source_type, provider, *aliases)
        return provider

    return decorator


def get_provider_class(source_type: str) -> ProviderClass:
    provider_cls = provider_registry.get(source_type)
    if provider_cls is None:
        raise SourceIngestionError(f"Unsupported source_type: {source_type}")
    return provider_cls


def build_provider(source: Source, keyword: Keyword) -> BaseProvider:
    provider_cls = get_provider_class(source.source_type)
    return provider_cls(source=source, keyword=keyword)

