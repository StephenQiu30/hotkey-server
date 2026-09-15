"""Explicit model registry used only for migration metadata discovery."""

from audit import models as audit
from collection import models as collection
from contents import models as contents
from db.base import Base
from events import models as events
from evidence import models as evidence
from identity import models as identity
from jobs import models as jobs
from monitors import models as monitors

__all__ = [
    "Base",
    "audit",
    "collection",
    "contents",
    "evidence",
    "events",
    "identity",
    "jobs",
    "monitors",
]
