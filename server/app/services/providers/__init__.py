from .base import BaseProvider, Candidate, SourceIngestionError
from .registry import build_provider, get_provider_class, normalize_source_type, provider_registry, register_provider

# Import providers to register implementations.
from . import bing  # noqa: F401
from . import bilibili  # noqa: F401
from . import github_trending  # noqa: F401
from . import hacker_news  # noqa: F401
from . import rss  # noqa: F401
from . import sogou  # noqa: F401
from . import x_twitter  # noqa: F401

__all__ = [
    "BaseProvider",
    "Candidate",
    "SourceIngestionError",
    "build_provider",
    "get_provider_class",
    "normalize_source_type",
    "provider_registry",
    "register_provider",
    "rss",
    "github_trending",
    "hacker_news",
    "bing",
    "bilibili",
    "x_twitter",
    "sogou",
]
