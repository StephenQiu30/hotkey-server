"""Explicit model registry used only for migration metadata discovery."""

from audit import models as audit
from db.base import Base
from identity import models as identity
from jobs import models as jobs
from monitors import models as monitors

__all__ = ["Base", "audit", "identity", "monitors", "jobs"]
