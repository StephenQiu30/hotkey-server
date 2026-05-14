from __future__ import annotations

from typing import Callable
from typing import TypeVar

from .base import BaseLLMProvider

ProviderClass = type[BaseLLMProvider]
_ProviderDecorator = Callable[[ProviderClass], ProviderClass]


class LLMProviderRegistry:
    def __init__(self) -> None:
        self._providers: dict[str, ProviderClass] = {}

    def register(self, provider_key: str, provider: ProviderClass) -> None:
        self._providers[provider_key.strip().lower()] = provider

    def get(self, provider_key: str) -> ProviderClass | None:
        return self._providers.get(provider_key.strip().lower())

    def __contains__(self, provider_key: str) -> bool:
        return self._providers.get(provider_key.strip().lower()) is not None


provider_registry = LLMProviderRegistry()


def register_llm_provider(key: str) -> _ProviderDecorator:
    def decorator(provider: ProviderClass) -> ProviderClass:
        provider_registry.register(key, provider)
        return provider

    return decorator


def build_provider(key: str) -> BaseLLMProvider:
    provider_cls = provider_registry.get(key)
    if provider_cls is None:
        raise RuntimeError(f"Unsupported ai provider: {key}")
    return provider_cls()
