// Package model contains database record declarations and the schema mapping
// used by architecture tests. It deliberately has no pgx, GORM, HTTP or domain
// dependency; PLAN-002 supplies the database runtime and repository adapters.
package model

import (
	"encoding/json"
	"time"
)

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

func (r *Record) GetID() int64       { return r.ID }
func (r *Record) GetVersion() int64  { return r.Version }
func (r *Record) SetVersion(v int64) { r.Version = v }

type OperationalRecord struct {
	ID int64
}

func (r *OperationalRecord) GetID() int64 { return r.ID }

type Spec struct {
	Table     string
	Lifecycle Lifecycle
	Columns   []string
}

// DeletionPolicy describes the only generic delete operation safe for a
// table. State archival is intentionally left to the owning module because a
// blanket repository cannot safely infer its domain transition.
type DeletionPolicy string

const (
	DeletionSoft     DeletionPolicy = "soft"
	DeletionHard     DeletionPolicy = "hard"
	DeletionRetained DeletionPolicy = "retained"
)

// Persistence is the database-specific metadata consumed by repository
// adapters. It is derived from the authoritative record mapping rather than a
// second per-table manifest.
type Persistence struct {
	Table         string
	KeyColumn     string
	VersionColumn string
	Deletion      DeletionPolicy
	AllowedSort   []string
	CursorFields  []string
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
	Name, Description, Status                      string
	DraftConfigVersionID, PublishedConfigVersionID *int64
}
type MonitorConfigVersion struct {
	Record
	MonitorID, Revision         int64
	State, Timezone, ConfigHash string
	PublishedAt                 *time.Time
}
type MonitorRule struct {
	Record
	ConfigVersionID int64
}
type MonitorSource struct {
	Record
	ConfigVersionID, SourceConnectionID int64
	QuerySignature                      string
}
type SourceAuthor struct {
	Record
	SourceConnectionID int64
}
type Content struct {
	Record
	SourceConnectionID                             int64
	ExternalID, DedupeKey                          string
	DedupeReason, DedupeVersion                    *string
	ViewCount, LikeCount, CommentCount, ShareCount *int64
}
type ContentAsset struct {
	Record
	ContentID int64
	ObjectKey string
}
type MonitorMatch struct {
	Record
	MonitorID, MonitorConfigVersionID, ContentID int64
	InputHash, ScoringVersion                    string
	EmbeddingModelProfileID                      *int64
	EmbeddingModelProfileVersion                 *int64
	EmbeddingModelVersion                        *string
	ReviewAIRunID                                *int64
}
type MonitorMatchFeedback struct {
	Record
	MonitorID, MonitorConfigVersionID, ContentID, ActorUserID int64
	MonitorMatchID                                            *int64
}
type MonitorFeedbackSuggestion struct {
	Record
	MonitorID, MonitorConfigVersionID int64
	SuggestionType, Status            string
	ReviewedByUserID                  *int64
}
type Event struct {
	Record
	EventKey string
}
type EventContent struct {
	Record
	EventID, ContentID int64
}
type EventClusteringDecision struct {
	OperationalRecord
	ContentID, CandidateEventID                                                               int64
	CandidateEventKey, ClusteringVersion, FeatureInputHash, Channel, Decision, DecisionOrigin string
}
type EventGovernanceAudit struct {
	OperationalRecord
	EventID, ActorUserID, SourceEventID, TargetEventID int64
	Action, ReasonCode, FromStatus, ToStatus           string
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
	Name, TaskType, Provider, ModelName, ModelVersion string
	CredentialRef                                     *string
	EmbeddingDimensions                               *int16
	TimeoutSeconds                                    int
	MaxAttempts                                       int16
	FallbackPriority                                  int16
	Enabled                                           bool
	DeletedAt                                         *time.Time
}
type RetentionPolicy struct {
	Record
	DataClass string
}
type AuthSession struct {
	OperationalRecord
	UserID            int64
	FamilyID          string
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
}
type AuthRefreshToken struct {
	OperationalRecord
	SessionID int64
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}
type SourceCheckpoint struct {
	OperationalRecord
	MonitorSourceID     int64
	QueryHash           string
	CursorValue         *string
	ETag                *string
	LastModified        *string
	HighWatermark       *time.Time
	LastSuccessfulRunID *int64
	LastFetchedAt       *time.Time
	NextPollAt          time.Time
	ConsecutiveFailures int
	Version             int64
	UpdatedAt           time.Time
}
type CollectionRun struct {
	OperationalRecord
	SourceConnectionID                             int64
	QuerySignature, TriggerType, Status            string
	RequestCursor, NextCursor, ETag, LastModified  *string
	RetryAfter                                     *time.Time
	PageCount                                      int
	WindowStart, WindowEnd, ScheduledAt, CreatedAt time.Time
	StartedAt, FinishedAt                          *time.Time
	CandidateCount, AcceptedCount, RejectedCount   int64
	ErrorCode                                      *string
	UpdatedAt                                      time.Time
}
type CollectionRunTarget struct {
	OperationalRecord
	CollectionRunID, MonitorSourceID, MonitorConfigVersionID int64
	TargetStatus                                             string
	CandidateCount, AcceptedCount, RejectedCount             int64
	ErrorCode                                                *string
	CreatedAt, UpdatedAt                                     time.Time
}
type CollectionRunItem struct {
	OperationalRecord
	RunID, SourceConnectionID                       int64
	SourceCode, ExternalID, ContentType             string
	CapturedItemVersion, PayloadHash                string
	CapturedItem                                    json.RawMessage
	RawPayloadDisposition, Outcome, IngestionStatus string
	ContentID                                       *int64
	ReasonCode, IngestionErrorCode                  *string
	ObservedAt, CreatedAt                           time.Time
}
type CollectionRunTargetItem struct {
	OperationalRecord
	CollectionRunID, CollectionRunTargetID, CollectionRunItemID int64
	Outcome                                                     string
	ReasonCode                                                  *string
	CreatedAt                                                   time.Time
}
type ContentMetricSnapshot struct {
	OperationalRecord
	ContentID                                      int64
	ViewCount, LikeCount, CommentCount, ShareCount *int64
}
type EventMetricSnapshot struct {
	OperationalRecord
	EventID int64
}
type AIRun struct {
	OperationalRecord
	TaskType, TargetType, PromptVersion, SchemaVersion, ModelVersion, ParametersVersion, InputSchemaVersion string
	TargetID, ModelProfileID, ModelProfileVersion                                                           int64
	InputHash, EvidenceSetHash, ReuseKey                                                                    string
	Attempt, MaxAttempts                                                                                    int16
	RepairAttempted                                                                                         bool
	RetryAfter, LeaseExpiresAt                                                                              *time.Time
	ErrorCode                                                                                               *int
	BudgetDay                                                                                               time.Time
}
type AIBudgetLedger struct {
	OperationalRecord
	ModelProfileID int64
	BudgetDay      time.Time
	OverageBlocked bool
	UpdatedAt      time.Time
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
	{"monitors", LifecycleBusiness, []string{"id", "version", "name", "status", "draft_config_version_id", "published_config_version_id", "deleted_at"}},
	{"monitor_config_versions", LifecycleBusiness, []string{"id", "version", "monitor_id", "revision", "state", "config_hash", "published_at"}},
	{"monitor_rules", LifecycleBusiness, []string{"id", "version", "config_version_id", "rule_type", "value"}},
	{"monitor_sources", LifecycleBusiness, []string{"id", "version", "config_version_id", "source_connection_id", "query_signature"}},
	{"source_authors", LifecycleBusiness, []string{"id", "source_connection_id", "external_id"}},
	{"contents", LifecycleBusiness, []string{"id", "source_connection_id", "external_id", "dedupe_key", "dedupe_reason", "dedupe_version", "view_count", "like_count", "comment_count", "share_count", "deleted_at"}},
	{"content_assets", LifecycleBusiness, []string{"id", "content_id", "object_key", "object_status"}},
	{"monitor_matches", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "input_hash", "scoring_version", "final_score", "decision", "decision_origin", "embedding_model_profile_id", "embedding_model_profile_version", "embedding_model_version", "review_ai_run_id"}},
	{"monitor_match_feedbacks", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "monitor_match_id", "actor_user_id", "feedback_type"}},
	{"monitor_feedback_suggestions", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "suggestion_type", "value", "support_count", "status", "reviewed_by_user_id"}},
	{"events", LifecycleBusiness, []string{"id", "event_key", "lifecycle_status", "deleted_at"}},
	{"event_contents", LifecycleBusiness, []string{"id", "event_id", "content_id", "membership_score"}},
	{"event_clustering_decisions", LifecycleOperational, []string{"id", "content_id", "candidate_event_id", "candidate_event_key", "clustering_version", "feature_input_hash", "channel", "candidate_rank", "membership_score", "decision", "decision_origin", "feature_snapshot", "evidence_content_ids", "actor_user_id", "created_at"}},
	{"event_governance_audits", LifecycleOperational, []string{"id", "event_id", "action", "actor_user_id", "reason_code", "from_status", "to_status", "source_event_id", "target_event_id", "expected_version", "metadata", "created_at"}},
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
	{"ai_model_profiles", LifecycleBusiness, []string{"id", "version", "name", "task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions", "timeout_seconds", "max_attempts", "max_cost", "daily_budget", "fallback_priority", "enabled", "deleted_at"}},
	{"retention_policies", LifecycleBusiness, []string{"id", "data_class", "retention_days", "action"}},
	{"auth_sessions", LifecycleOperational, []string{"id", "user_id", "family_id", "absolute_expires_at", "revoked_at"}},
	{"auth_refresh_tokens", LifecycleOperational, []string{"id", "session_id", "token_hash", "expires_at", "used_at", "revoked_at"}},
	{"source_checkpoints", LifecycleOperational, []string{"id", "monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"}},
	{"collection_runs", LifecycleOperational, []string{"id", "source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "status", "updated_at"}},
	{"collection_run_targets", LifecycleOperational, []string{"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status", "updated_at"}},
	{"collection_run_items", LifecycleOperational, []string{"id", "run_id", "source_connection_id", "source_code", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "outcome", "observed_at"}},
	{"collection_run_target_items", LifecycleOperational, []string{"id", "collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"}},
	{"content_metric_snapshots", LifecycleOperational, []string{"id", "content_id", "captured_at", "view_count", "like_count", "comment_count", "share_count"}},
	{"event_metric_snapshots", LifecycleOperational, []string{"id", "event_id", "captured_at"}},
	{"ai_runs", LifecycleOperational, []string{"id", "task_type", "target_type", "target_id", "model_profile_id", "model_profile_version", "model_version", "prompt_version", "input_schema_version", "schema_version", "parameters_version", "input_hash", "evidence_set_hash", "reuse_key", "attempt", "max_attempts", "repair_attempted", "retry_after", "error_code", "budget_day", "reserved_cost", "lease_expires_at", "status"}},
	{"ai_run_evidences", LifecycleOperational, []string{"id", "ai_run_id", "content_id"}},
	{"ai_budget_ledgers", LifecycleOperational, []string{"id", "model_profile_id", "budget_day", "reserved_cost", "settled_cost", "overage_blocked", "updated_at"}},
	{"content_embeddings", LifecycleOperational, []string{"id", "content_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"}},
	{"monitor_embeddings", LifecycleOperational, []string{"id", "monitor_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "query_text", "embedding", "active"}},
	{"event_embeddings", LifecycleOperational, []string{"id", "event_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"}},
	{"topic_embeddings", LifecycleOperational, []string{"id", "topic_id", "model_profile_id", "model_profile_version", "ai_run_id", "model_version", "input_hash", "embedding", "active"}},
	{"knowledge_revisions", LifecycleOperational, []string{"id", "document_id", "revision_no"}},
	{"vault_sync_runs", LifecycleOperational, []string{"id", "run_type", "status"}},
	{"report_deliveries", LifecycleOperational, []string{"id", "report_id", "subscription_id", "idempotency_key", "status"}},
	{"delivery_attempts", LifecycleOperational, []string{"id", "delivery_id", "attempt_no", "status"}},
	{"audit_logs", LifecycleOperational, []string{"id", "action", "resource_type", "result"}},
}

func All() []Spec { return append([]Spec(nil), specs...) }

// PersistenceFor returns the table metadata needed by controlled generic
// CRUD. It intentionally authorizes only the stable id ordering until a
// module supplies a use-case-specific query and index.
func PersistenceFor(table string) (Persistence, bool) {
	for _, spec := range specs {
		if spec.Table != table {
			continue
		}
		persistence := Persistence{
			Table:        spec.Table,
			KeyColumn:    "id",
			AllowedSort:  []string{"id"},
			CursorFields: []string{"id"},
		}
		if spec.Lifecycle == LifecycleBusiness {
			persistence.VersionColumn = "version"
		}
		switch {
		case spec.Table == "monitor_config_versions" || spec.Table == "monitor_rules" || spec.Table == "monitor_sources":
			persistence.Deletion = DeletionRetained
			if spec.Table == "monitor_config_versions" {
				persistence.AllowedSort = []string{"revision", "id"}
				persistence.CursorFields = []string{"revision", "id"}
			}
		case spec.Lifecycle == LifecycleOperational:
			persistence.Deletion = DeletionRetained
		case hasColumn(spec.Columns, "deleted_at"):
			persistence.Deletion = DeletionSoft
		default:
			persistence.Deletion = DeletionHard
		}
		return persistence, true
	}
	return Persistence{}, false
}

func hasColumn(columns []string, wanted string) bool {
	for _, column := range columns {
		if column == wanted {
			return true
		}
	}
	return false
}
