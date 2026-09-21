from db.base import Base
from evidence.models import (
    CleanupTarget,
    DeletionDirective,
    EvidenceResource,
    RetentionPolicy,
    SourceAccessPolicy,
)
from identity.models import IdentitySession, IdentityUser
from jobs.models import (
    Job,
    JobAttempt,
    OutboxMessage,
    ProcessedMessage,
    ResourceBudgetPolicy,
    ResourceBudgetReservation,
    ResourceBudgetWindow,
    ResourceComponentPolicy,
    ResourceUsageAttempt,
)

# Import each domain's models here for runtime mapping and clean-database verification.
# DDL ownership remains exclusively in database/schema.sql.
metadata = Base.metadata

__all__ = [
    "CleanupTarget",
    "DeletionDirective",
    "EvidenceResource",
    "IdentitySession",
    "IdentityUser",
    "Job",
    "JobAttempt",
    "OutboxMessage",
    "ProcessedMessage",
    "ResourceBudgetPolicy",
    "ResourceBudgetReservation",
    "ResourceBudgetWindow",
    "ResourceComponentPolicy",
    "ResourceUsageAttempt",
    "RetentionPolicy",
    "SourceAccessPolicy",
    "metadata",
]
