// Package model contains database record declarations and the schema mapping
// used by architecture tests. It deliberately has no pgx, GORM, HTTP or domain
// dependency; PLAN-002 supplies the database runtime and repository adapters.
package model

import "time"

type Lifecycle string

const (
	LifecycleBusiness    Lifecycle = "business"
	LifecycleOperational Lifecycle = "operational"
)

type Record struct {
	ID        int64
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OperationalRecord struct {
	ID int64
}

type Spec struct {
	Table     string
	Lifecycle Lifecycle
	Columns   []string
}

// The strongly named records make table ownership explicit before GORM is
// introduced. Schema remains the executable source for SQL types and defaults.
type User struct {
	Record
	Email, PasswordHash, DisplayName, Role, Status string
}
type UserPreference struct {
	Record
	UserID int64
}
type SourceConnection struct {
	Record
	SourceType, Name, Endpoint string
}
type Monitor struct {
	Record
	Name, Status string
}
type MonitorRule struct {
	Record
	MonitorID int64
}
type MonitorSource struct {
	Record
	MonitorID, SourceConnectionID int64
}
type SourceAuthor struct {
	Record
	SourceConnectionID int64
}
type Content struct {
	Record
	SourceConnectionID    int64
	ExternalID, DedupeKey string
}
type ContentAsset struct {
	Record
	ContentID int64
	ObjectKey string
}
type MonitorMatch struct {
	Record
	MonitorID, ContentID int64
}
type Event struct {
	Record
	EventKey string
}
type EventContent struct {
	Record
	EventID, ContentID int64
}
type MonitorEvent struct {
	Record
	MonitorID, EventID int64
}
type Entity struct {
	Record
	EntityKey string
}
type EntityAlias struct {
	Record
	EntityID int64
}
type EventEntity struct {
	Record
	EventID, EntityID int64
}
type EventClaim struct {
	Record
	EventID   int64
	ClaimHash string
}
type ClaimEvidence struct {
	Record
	ClaimID, ContentID int64
}
type Topic struct {
	Record
	TopicKey string
}
type TopicEvent struct {
	Record
	TopicID, EventID int64
}
type TopicEntity struct {
	Record
	TopicID, EntityID int64
}
type TopicRelation struct {
	Record
	FromTopicID, ToTopicID int64
}
type EntityRelation struct {
	Record
	FromEntityID, ToEntityID int64
}
type KnowledgeDocument struct {
	Record
	VaultPath string
}
type KnowledgeChangeProposal struct {
	Record
	DocumentID int64
}
type KnowledgeAnnotation struct {
	Record
	DocumentID int64
}
type Report struct {
	Record
	ReportType string
}
type ReportItem struct {
	Record
	ReportID, EventID int64
}
type ReportSubscription struct {
	Record
	UserID int64
}
type AIModelProfile struct {
	Record
	Name string
}
type RetentionPolicy struct {
	Record
	DataClass string
}
type AuthSession struct {
	OperationalRecord
	UserID    int64
	TokenHash string
}
type SourceCheckpoint struct {
	OperationalRecord
	MonitorSourceID int64
}
type CollectionRun struct {
	OperationalRecord
	MonitorSourceID int64
	IdempotencyKey  string
}
type CollectionRunItem struct {
	OperationalRecord
	RunID int64
}
type ContentMetricSnapshot struct {
	OperationalRecord
	ContentID int64
}
type EventMetricSnapshot struct {
	OperationalRecord
	EventID int64
}
type AIRun struct {
	OperationalRecord
	TargetID int64
}
type AIRunEvidence struct {
	OperationalRecord
	AIRunID, ContentID int64
}
type ContentEmbedding struct {
	OperationalRecord
	ContentID int64
}
type MonitorEmbedding struct {
	OperationalRecord
	MonitorID int64
}
type EventEmbedding struct {
	OperationalRecord
	EventID int64
}
type TopicEmbedding struct {
	OperationalRecord
	TopicID int64
}
type KnowledgeRevision struct {
	OperationalRecord
	DocumentID int64
}
type VaultSyncRun struct{ OperationalRecord }
type ReportDelivery struct {
	OperationalRecord
	ReportID, SubscriptionID int64
	IdempotencyKey           string
}
type DeliveryAttempt struct {
	OperationalRecord
	DeliveryID int64
}
type AuditLog struct {
	OperationalRecord
	Action string
}

var specs = []Spec{
	{"users", LifecycleBusiness, []string{"id", "version", "email", "password_hash", "role", "status", "deleted_at"}},
	{"user_preferences", LifecycleBusiness, []string{"id", "user_id", "timezone", "preferences"}},
	{"source_connections", LifecycleBusiness, []string{"id", "source_type", "name", "endpoint", "deleted_at"}},
	{"monitors", LifecycleBusiness, []string{"id", "name", "status", "relevance_threshold", "deleted_at"}},
	{"monitor_rules", LifecycleBusiness, []string{"id", "monitor_id", "rule_type", "value", "deleted_at"}},
	{"monitor_sources", LifecycleBusiness, []string{"id", "monitor_id", "source_connection_id"}},
	{"source_authors", LifecycleBusiness, []string{"id", "source_connection_id", "external_id"}},
	{"contents", LifecycleBusiness, []string{"id", "source_connection_id", "external_id", "dedupe_key", "deleted_at"}},
	{"content_assets", LifecycleBusiness, []string{"id", "content_id", "object_key", "object_status"}},
	{"monitor_matches", LifecycleBusiness, []string{"id", "monitor_id", "content_id", "final_score"}},
	{"events", LifecycleBusiness, []string{"id", "event_key", "lifecycle_status", "deleted_at"}},
	{"event_contents", LifecycleBusiness, []string{"id", "event_id", "content_id", "membership_score"}},
	{"monitor_events", LifecycleBusiness, []string{"id", "monitor_id", "event_id", "final_score"}},
	{"entities", LifecycleBusiness, []string{"id", "entity_key", "entity_type", "deleted_at"}},
	{"entity_aliases", LifecycleBusiness, []string{"id", "entity_id", "normalized_alias"}},
	{"event_entities", LifecycleBusiness, []string{"id", "event_id", "entity_id", "role"}},
	{"event_claims", LifecycleBusiness, []string{"id", "event_id", "claim_hash", "status"}},
	{"claim_evidences", LifecycleBusiness, []string{"id", "claim_id", "content_id", "stance"}},
	{"topics", LifecycleBusiness, []string{"id", "topic_key", "status", "deleted_at"}},
	{"topic_events", LifecycleBusiness, []string{"id", "topic_id", "event_id", "relation_type"}},
	{"topic_entities", LifecycleBusiness, []string{"id", "topic_id", "entity_id", "relation_type"}},
	{"topic_relations", LifecycleBusiness, []string{"id", "from_topic_id", "to_topic_id", "relation_type"}},
	{"entity_relations", LifecycleBusiness, []string{"id", "from_entity_id", "to_entity_id", "relation_type"}},
	{"knowledge_documents", LifecycleBusiness, []string{"id", "document_type", "vault_path", "revision_no"}},
	{"knowledge_change_proposals", LifecycleBusiness, []string{"id", "document_id", "change_type", "status"}},
	{"knowledge_annotations", LifecycleBusiness, []string{"id", "document_id", "annotation_type", "deleted_at"}},
	{"reports", LifecycleBusiness, []string{"id", "report_type", "period_start", "status", "deleted_at"}},
	{"report_items", LifecycleBusiness, []string{"id", "report_id", "event_id", "rank"}},
	{"report_subscriptions", LifecycleBusiness, []string{"id", "user_id", "channel", "deleted_at"}},
	{"ai_model_profiles", LifecycleBusiness, []string{"id", "name", "task_type", "deleted_at"}},
	{"retention_policies", LifecycleBusiness, []string{"id", "data_class", "retention_days", "action"}},
	{"auth_sessions", LifecycleOperational, []string{"id", "user_id", "token_hash", "expires_at"}},
	{"source_checkpoints", LifecycleOperational, []string{"id", "monitor_source_id", "next_poll_at"}},
	{"collection_runs", LifecycleOperational, []string{"id", "monitor_source_id", "idempotency_key", "status"}},
	{"collection_run_items", LifecycleOperational, []string{"id", "run_id", "external_id", "outcome"}},
	{"content_metric_snapshots", LifecycleOperational, []string{"id", "content_id", "captured_at"}},
	{"event_metric_snapshots", LifecycleOperational, []string{"id", "event_id", "captured_at"}},
	{"ai_runs", LifecycleOperational, []string{"id", "task_type", "target_id", "input_hash", "status"}},
	{"ai_run_evidences", LifecycleOperational, []string{"id", "ai_run_id", "content_id"}},
	{"content_embeddings", LifecycleOperational, []string{"id", "content_id", "embedding", "active"}},
	{"monitor_embeddings", LifecycleOperational, []string{"id", "monitor_id", "embedding", "active"}},
	{"event_embeddings", LifecycleOperational, []string{"id", "event_id", "embedding", "active"}},
	{"topic_embeddings", LifecycleOperational, []string{"id", "topic_id", "embedding", "active"}},
	{"knowledge_revisions", LifecycleOperational, []string{"id", "document_id", "revision_no"}},
	{"vault_sync_runs", LifecycleOperational, []string{"id", "run_type", "status"}},
	{"report_deliveries", LifecycleOperational, []string{"id", "report_id", "subscription_id", "idempotency_key", "status"}},
	{"delivery_attempts", LifecycleOperational, []string{"id", "delivery_id", "attempt_no", "status"}},
	{"audit_logs", LifecycleOperational, []string{"id", "action", "resource_type", "result"}},
}

func All() []Spec { return append([]Spec(nil), specs...) }
