from ai.models import AiCall
from analysis.models import ContentAnnotation
from connections.models import (
    SourceCapabilityEvidence,
    SourceConnection,
    SourceConnectionVersion,
)
from content.models import (
    ContentDiscovery,
    ContentObservation,
    ContentRecord,
    ContentThread,
    ContentVersion,
    ContentVersionRelation,
    ContentVisibilityObservation,
    HotlistEntryRecord,
    HotlistSnapshot,
)
from db.base import Base
from evidence.models import (
    CleanupTarget,
    DeletionDirective,
    EvidenceResource,
    ProvenanceManifest,
    ProvenanceManifestItem,
    RetentionPolicy,
    SourceAccessPolicy,
)
from identity.models import IdentitySession, IdentityUser
from jobs.models import (
    CollectionDueWindow,
    CoverageWindow,
    Job,
    JobAttempt,
    JobStageAttempt,
    OutboxMessage,
    ProcessedMessage,
    ResourceBudgetPolicy,
    ResourceBudgetReservation,
    ResourceBudgetWindow,
    ResourceComponentPolicy,
    ResourceUsageAttempt,
)
from knowledge.models import KnowledgeExport
from monitors.models import (
    FollowedAccount,
    FollowedAccountAlias,
    MonitorSchedule,
    MonitorTopic,
    MonitorTopicVersion,
)
from notifications.models import NotificationDelivery, NotificationTarget
from reports.models import Report

# Import each domain's models here for runtime mapping and clean-database verification.
# DDL ownership remains exclusively in database/schema.sql.
metadata = Base.metadata

__all__ = [
    "AiCall",
    "CleanupTarget",
    "CollectionDueWindow",
    "ContentAnnotation",
    "ContentDiscovery",
    "ContentObservation",
    "ContentRecord",
    "ContentThread",
    "ContentVersion",
    "ContentVersionRelation",
    "ContentVisibilityObservation",
    "CoverageWindow",
    "DeletionDirective",
    "EvidenceResource",
    "FollowedAccount",
    "FollowedAccountAlias",
    "HotlistEntryRecord",
    "HotlistSnapshot",
    "IdentitySession",
    "IdentityUser",
    "Job",
    "JobAttempt",
    "JobStageAttempt",
    "KnowledgeExport",
    "MonitorSchedule",
    "MonitorTopic",
    "MonitorTopicVersion",
    "NotificationDelivery",
    "NotificationTarget",
    "OutboxMessage",
    "ProcessedMessage",
    "ProvenanceManifest",
    "ProvenanceManifestItem",
    "Report",
    "ResourceBudgetPolicy",
    "ResourceBudgetReservation",
    "ResourceBudgetWindow",
    "ResourceComponentPolicy",
    "ResourceUsageAttempt",
    "RetentionPolicy",
    "SourceAccessPolicy",
    "SourceCapabilityEvidence",
    "SourceConnection",
    "SourceConnectionVersion",
    "metadata",
]
