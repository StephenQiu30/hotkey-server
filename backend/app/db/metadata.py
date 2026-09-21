from db.base import Base
from identity.models import IdentitySession, IdentityUser
from jobs.models import Job, JobAttempt, OutboxMessage, ProcessedMessage

# Import each domain's models here for runtime mapping and clean-database verification.
# DDL ownership remains exclusively in database/schema.sql.
metadata = Base.metadata

__all__ = [
    "IdentitySession",
    "IdentityUser",
    "Job",
    "JobAttempt",
    "OutboxMessage",
    "ProcessedMessage",
    "metadata",
]
