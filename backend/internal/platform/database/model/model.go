// Package model contains database record declarations and the schema mapping
// used by architecture tests. It deliberately has no pgx, GORM, HTTP or domain
// dependency; PLAN-002 supplies the database runtime and repository adapters.
package model

import (
	"encoding/json"
	"strings"
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
type SourceCredential struct {
	OperationalRecord
	SourceConnectionID int64
}
type MetricCapabilityProfile struct {
	Record
	SourceType, ProfileVersion, Status string
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
type MonitorIntentDraft struct {
	ID, ResourceVersion, MonitorID, ConfigVersionID int64
}
type MonitorIntentDraftRevision struct {
	Record
	DraftID, MonitorID, ConfigVersionID, ResourceVersion int64
	Objective                                            string
}
type MonitorIntentClause struct {
	Record
	RevisionID, DraftID, ResourceVersion int64
	Operator, Field, Value               string
}
type MonitorIntentEntity struct {
	Record
	RevisionID, DraftID, ResourceVersion    int64
	CanonicalID, DisplayName, AmbiguityNote string
}
type MonitorIntentEntityAlias struct {
	Record
	EntityID, DraftID, ResourceVersion int64
	Alias                              string
}
type MonitorIntentExample struct {
	Record
	RevisionID, DraftID, ResourceVersion int64
	Label, Text                          string
}
type MonitorIntentAnalysisRun struct {
	OperationalRecord
	MonitorID, DraftID, DraftResourceVersion, RiverJobID int64
	Kind, InputHash, ProfileVersion, RequestHash, Status string
}
type MonitorIntentExpansionCandidate struct {
	Record
	DraftID, IntroducedResourceVersion int64
	OriginRunID                        *int64
	CandidateID, Value, Source, Risk   string
}
type MonitorIntentDraftCandidate struct {
	Record
	RevisionID, DraftID, ResourceVersion, CandidateRecordID int64
	ApprovalStatus                                          string
}
type MonitorIntentMutationReceipt struct {
	OperationalRecord
	MonitorID, DraftID, ExpectedResourceVersion, ResultResourceVersion int64
	MutationKind, IdempotencyKey, CommandFingerprint                   string
}
type MonitorIntentPreviewResult struct {
	RunID               int64
	EstimatedAlertCount int
}
type MonitorIntentPreviewSample struct {
	OperationalRecord
	RunID, DocumentVersionID int64
	Title, Decision          string
}
type MonitorIntentPreviewRecallSignal struct {
	OperationalRecord
	SampleID, RunID int64
	Channel         string
}
type MonitorIntentPreviewReason struct {
	OperationalRecord
	SampleID, RunID    int64
	ReasonType, Reason string
}
type MonitorIntentPreviewWarning struct {
	OperationalRecord
	RunID   int64
	Warning string
}
type MonitorCompiledProfile struct {
	Record
	MonitorID, ConfigVersionID, IntentRevisionID int64
	Purpose, Status, SemanticState               string
}
type MonitorCompiledClause struct {
	Record
	CompiledProfileID       int64
	Operator, Field, Origin string
}
type MonitorCompiledEntity struct {
	Record
	CompiledProfileID int64
	CanonicalID       string
}
type MonitorCompiledEntityAlias struct {
	Record
	CompiledEntityID, CompiledProfileID int64
	Alias                               string
}
type MonitorCompiledIntentEmbedding struct {
	OperationalRecord
	CompiledProfileID, ModelProfileID, ModelProfileVersion, AIRunID int64
}
type RelevanceDecisionProfile struct {
	Record
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion, Status string
}
type DocumentMatchDecision struct {
	OperationalRecord
	MonitorID, MonitorVersionID, CompiledProfileID                int64
	DocumentVersionID, RelevanceProfileID                         int64
	MatchingAlgorithmVersion, RerankerVersion, CalibrationVersion string
}
type DocumentMatchRecallSignal struct {
	OperationalRecord
	MatchDecisionID           int64
	Channel, AlgorithmVersion string
}
type DocumentMatchOverride struct {
	OperationalRecord
	MatchDecisionID, MonitorID, MonitorVersionID, DocumentVersionID int64
	Decision, PreviousEffectiveDecision, IdempotencyKey             string
}
type SourceAuthor struct {
	Record
	SourceConnectionID int64
}
type SourceRightsPolicy struct {
	Record
	SourceConnectionID                  *int64
	ScopeType, ScopeSubject, PolicyHash string
}
type SourceRightsDecision struct {
	OperationalRecord
	SourceConnectionID, PolicyID, PolicyRevision int64
	RetentionDays                                *int
	SupersedesDecisionID                         *int64
	PolicyScopeType, PolicyScopeSubject          string
	SubjectType, SubjectKey, InputDigest         string
	Action, Decision                             string
}
type EvidenceSnapshot struct {
	OperationalRecord
	SourceConnectionID, StoreRawRightsDecisionID, RetainRightsDecisionID int64
	SnapshotKey, ObjectKey, PayloadSHA256, CollectorProfileVersion       string
	LifecycleState                                                       string
}
type SourceObservation struct {
	Record
	SourceConnectionID           int64
	CollectionRunItemID          *int64
	ExternalID, UpstreamIdentity string
}
type SourceObservationEvidence struct {
	OperationalRecord
	SourceConnectionID, SourceObservationID, EvidenceSnapshotID int64
	LocatorType, LocatorValue                                   string
}
type DocumentVersionSearchIndex struct {
	OperationalRecord
	DocumentVersionID, SourceConnectionID, DerivedArtifactID int64
	LifecycleState                                           string
}
type DocumentVersionEmbedding struct {
	OperationalRecord
	DocumentVersionID, SourceConnectionID, ModelProfileID, AIRunID int64
	LifecycleState                                                 string
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
type ArchiveDocument struct {
	Record
	SourceConnectionID       int64
	DocumentKey              string
	CurrentDocumentVersionID *int64
}
type ArchiveDocumentVersion struct {
	Record
	DocumentID, SourceObservationID                                    int64
	DisplayPrivateRightsDecisionID                                     *int64
	QualityScore                                                       *float64
	VersionKey, ContentSHA256, ExtractorProfileVersion, LifecycleState string
}
type DocumentIdentityKey struct {
	Record
	SourceConnectionID, DocumentID int64
	IdentityKind, IdentityValue    string
}
type DerivedArtifact struct {
	OperationalRecord
	SourceConnectionID, DocumentVersionID                                     int64
	StoreDerivedRightsDecisionID, RetainRightsDecisionID                      int64
	ArtifactType, TransformerProfileSHA256, VaultRelativePath, LifecycleState string
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
type ReportItemSentence struct {
	Record
	ReportItemID, SourceSummarySentenceID int64
}
type ReportItemSentenceEvidence struct {
	OperationalRecord
	ReportItemSentenceID, ClaimEvidenceVersionID int64
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
	EventID                      int64
	CapturedAt                   time.Time
	HeatScore, TrendScore        float64
	SourceCount, ContentCount    int64
	HeatVersion, EvidenceSetHash string
}
type EventUpdate struct {
	OperationalRecord
	Version, EventID, SequenceNo int64
	Kind, IdempotencyKey         string
}
type AlertThread struct {
	Record
	MonitorID, MonitorConfigVersionID, MonitorRevision, EventID int64
	TriggerType, PolicyVersion, State, Severity                 string
}
type AlertOccurrence struct {
	OperationalRecord
	AlertThreadID, EventUpdateID int64
	Fingerprint                  string
}
type AlertStateAudit struct {
	OperationalRecord
	AlertThreadID                             int64
	ActorType, FromState, ToState, ReasonCode string
	ActorUserID                               *int64
}
type AlertEmailDelivery struct {
	OperationalRecord
	OccurrenceID                     int64
	IdempotencyKey, Severity, Status string
}
type AlertEmailAttempt struct {
	OperationalRecord
	DeliveryID int64
	AttemptNo  int
	Status     string
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
	{"source_credentials", LifecycleOperational, []string{"id", "source_connection_id", "key_version", "nonce", "ciphertext", "updated_at"}},
	{"metric_capability_profiles", LifecycleBusiness, []string{"id", "version", "source_type", "profile_version", "status", "published_at", "archived_at"}},
	{"monitors", LifecycleBusiness, []string{"id", "version", "name", "status", "draft_config_version_id", "published_config_version_id", "deleted_at"}},
	{"monitor_config_versions", LifecycleBusiness, []string{"id", "version", "monitor_id", "revision", "state", "config_hash", "published_at"}},
	{"monitor_rules", LifecycleBusiness, []string{"id", "version", "config_version_id", "rule_type", "value"}},
	{"monitor_sources", LifecycleBusiness, []string{"id", "version", "config_version_id", "source_connection_id", "query_signature"}},
	{"monitor_intent_drafts", LifecycleBusiness, []string{"id", "resource_version", "monitor_id", "config_version_id", "created_at", "updated_at"}},
	{"monitor_intent_draft_revisions", LifecycleBusiness, []string{"id", "version", "draft_id", "monitor_id", "config_version_id", "resource_version", "objective", "created_at"}},
	{"monitor_intent_clauses", LifecycleBusiness, []string{"id", "version", "revision_id", "draft_id", "resource_version", "ordinal", "operator", "field", "value"}},
	{"monitor_intent_entities", LifecycleBusiness, []string{"id", "version", "revision_id", "draft_id", "resource_version", "ordinal", "canonical_id", "display_name", "ambiguity_note"}},
	{"monitor_intent_entity_aliases", LifecycleBusiness, []string{"id", "version", "entity_id", "draft_id", "resource_version", "ordinal", "alias"}},
	{"monitor_intent_examples", LifecycleBusiness, []string{"id", "version", "revision_id", "draft_id", "resource_version", "ordinal", "label", "example_text"}},
	{"monitor_intent_analysis_runs", LifecycleOperational, []string{"id", "monitor_id", "draft_id", "draft_resource_version", "kind", "input_hash", "profile_version", "sample_limit", "request_hash", "idempotency_key", "river_job_id", "status", "queued_at", "started_at", "completed_at", "invalidated_at", "failure_reason", "result_fingerprint"}},
	{"monitor_intent_expansion_candidates", LifecycleBusiness, []string{"id", "version", "draft_id", "introduced_resource_version", "candidate_id", "origin_run_id", "candidate_value", "source", "reason", "model_version", "prompt_version", "input_hash", "similarity", "risk"}},
	{"monitor_intent_draft_candidates", LifecycleBusiness, []string{"id", "version", "revision_id", "draft_id", "resource_version", "candidate_record_id", "ordinal", "approval_status", "reviewer_user_id", "reviewed_at", "review_note"}},
	{"monitor_intent_mutation_receipts", LifecycleOperational, []string{"id", "monitor_id", "draft_id", "mutation_kind", "idempotency_key", "command_fingerprint", "expected_resource_version", "result_resource_version", "created_at"}},
	{"monitor_intent_preview_results", LifecycleOperational, []string{"run_id", "estimated_alert_count", "created_at"}},
	{"monitor_intent_preview_samples", LifecycleOperational, []string{"id", "run_id", "ordinal", "document_version_id", "title", "decision"}},
	{"monitor_intent_preview_recall_signals", LifecycleOperational, []string{"id", "sample_id", "run_id", "ordinal", "channel", "rank", "score"}},
	{"monitor_intent_preview_reasons", LifecycleOperational, []string{"id", "sample_id", "run_id", "ordinal", "reason_type", "reason"}},
	{"monitor_intent_preview_warnings", LifecycleOperational, []string{"id", "run_id", "ordinal", "warning"}},
	{"monitor_compiled_profiles", LifecycleBusiness, []string{"id", "version", "monitor_id", "purpose", "config_version_id", "monitor_version_id", "source_preview_compiled_profile_id", "preview_run_id", "draft_id", "draft_resource_version", "intent_revision_id", "compiler_version", "matching_algorithm_version", "lexical_algorithm_version", "semantic_algorithm_version", "structured_algorithm_version", "search_normalization_profile_version", "semantic_state", "semantic_unavailable_reason", "status", "profile_hash", "ready_at", "retired_at", "created_at"}},
	{"monitor_compiled_clauses", LifecycleBusiness, []string{"id", "version", "compiled_profile_id", "ordinal", "operator", "field", "value", "normalized_value", "origin", "created_at"}},
	{"monitor_compiled_entities", LifecycleBusiness, []string{"id", "version", "compiled_profile_id", "ordinal", "canonical_id", "created_at"}},
	{"monitor_compiled_entity_aliases", LifecycleBusiness, []string{"id", "version", "compiled_entity_id", "compiled_profile_id", "ordinal", "alias", "normalized_alias", "created_at"}},
	{"monitor_compiled_intent_embeddings", LifecycleOperational, []string{"id", "compiled_profile_id", "config_version_id", "model_profile_id", "model_profile_version", "model_version", "input_hash", "embedding", "ai_run_id", "created_at"}},
	{"relevance_evaluation_runs", LifecycleOperational, []string{"id", "version", "dataset_version", "dataset_hash", "family_isolation_hash", "annotation_protocol_version", "annotation_guideline_sha256", "split_strategy_version", "annotator_count", "agreement_metric", "agreement_score", "time_boundary", "sample_window_start", "sample_window_end", "matching_algorithm_version", "reranker_version", "calibration_version", "calibration_slope", "calibration_intercept", "reject_threshold", "accept_threshold", "sample_count", "positive_count", "negative_count", "recall_at_100", "precision_score", "recall_score", "expected_calibration_error", "brier_score", "precision_wilson_lower", "hard_negative_count", "hard_negative_passed", "status", "evaluated_by_user_id", "evaluated_at", "created_at"}},
	{"relevance_evaluation_slices", LifecycleOperational, []string{"id", "version", "evaluation_run_id", "dimension", "value", "sample_count", "positive_count", "negative_count", "precision_score", "recall_score", "passed", "created_at"}},
	{"relevance_decision_profiles", LifecycleBusiness, []string{"id", "version", "profile_name", "matching_algorithm_version", "reranker_version", "calibration_version", "status", "reject_threshold", "accept_threshold", "calibration_slope", "calibration_intercept", "evaluation_sample_count", "evaluation_run_id", "created_by_user_id", "activated_by_user_id", "activated_at", "rolled_back_by_user_id", "rolled_back_at", "created_at"}},
	{"decision_quality_evaluation_runs", LifecycleOperational, []string{"id", "version", "module", "profile_version", "dataset_version", "dataset_sha256", "annotation_protocol_version", "annotation_guideline_sha256", "split_strategy_version", "family_isolation_sha256", "event_isolation_sha256", "annotator_count", "agreement_metric", "agreement_score", "time_boundary", "sample_count", "positive_count", "negative_count", "precision_score", "recall_score", "precision_wilson_lower", "false_merge_rate", "pairwise_precision", "b_cubed_f1", "ceaf_e", "cluster_count_ratio", "locator_accuracy", "provenance_completeness", "evidence_relation_macro_f1", "hotspot_precision", "median_discovery_delay_seconds", "passed", "reason_codes", "evaluated_by_user_id", "evaluated_at", "created_at"}},
	{"decision_quality_evaluation_slices", LifecycleOperational, []string{"id", "version", "evaluation_run_id", "module", "dimension", "value", "sample_count", "precision_score", "recall_score", "passed", "created_at"}},
	{"decision_quality_profiles", LifecycleBusiness, []string{"id", "version", "module", "profile_version", "status", "evaluation_run_id", "created_by_user_id", "activated_by_user_id", "activated_at", "rolled_back_by_user_id", "rolled_back_at", "created_at", "updated_at"}},
	{"document_match_decisions", LifecycleOperational, []string{"id", "version", "monitor_id", "monitor_version_id", "compiled_profile_id", "document_version_id", "relevance_profile_id", "matching_algorithm_version", "reranker_version", "calibration_version", "rrf_score", "relevance_probability", "decision", "degraded", "reason_codes", "input_hash", "decided_at"}},
	{"document_match_recall_signals", LifecycleOperational, []string{"id", "version", "match_decision_id", "ordinal", "channel", "rank", "raw_score", "algorithm_version", "created_at"}},
	{"document_match_overrides", LifecycleOperational, []string{"id", "version", "match_decision_id", "sequence_no", "monitor_id", "monitor_version_id", "document_version_id", "previous_effective_decision", "decision", "reason_code", "note", "actor_user_id", "idempotency_key", "command_fingerprint", "created_at"}},
	{"source_authors", LifecycleBusiness, []string{"id", "source_connection_id", "external_id"}},
	{"source_rights_policies", LifecycleBusiness, []string{"id", "version", "recorded_by_user_id", "idempotency_key", "command_fingerprint", "source_connection_id", "scope_type", "scope_subject", "policy_revision", "policy_hash"}},
	{"source_rights_decision_batches", LifecycleOperational, []string{"id", "version", "source_connection_id", "policy_id", "expected_policy_version", "subject_type", "subject_key", "input_digest", "recorded_by_user_id", "idempotency_key", "command_fingerprint", "decision_count"}},
	{"source_rights_decisions", LifecycleOperational, []string{"id", "decision_batch_id", "source_connection_id", "policy_id", "policy_revision", "policy_scope_type", "policy_scope_subject", "priority_rank", "basis_summary", "subject_type", "subject_key", "input_digest", "action", "decision", "effective_from", "retention_days", "supersedes_decision_id"}},
	{"evidence_snapshots", LifecycleOperational, []string{"id", "source_connection_id", "store_raw_rights_decision_id", "retain_rights_decision_id", "snapshot_key", "object_key", "payload_sha256", "collector_profile_version", "retention_until", "lifecycle_state"}},
	{"evidence_retention_exceptions", LifecycleBusiness, []string{"id", "evidence_snapshot_id", "approved_by_user_id", "approval_basis", "approved_at", "expires_at", "revoked_by_user_id", "revoked_at", "revocation_basis", "created_at"}},
	{"evidence_deletion_audits", LifecycleOperational, []string{"id", "evidence_snapshot_id", "source_connection_id", "retention_policy_id", "retention_policy_version", "attempt_no", "event_type", "object_key", "payload_sha256", "reason_code", "already_missing", "occurred_at", "created_at"}},
	{"source_observations", LifecycleBusiness, []string{"id", "version", "source_connection_id", "collection_run_item_id", "external_id", "upstream_identity", "body_origin", "completeness"}},
	{"source_observation_evidences", LifecycleOperational, []string{"id", "source_connection_id", "source_observation_id", "evidence_snapshot_id", "usage", "locator_type", "locator_value", "selected_payload_sha256"}},
	{"source_parties", LifecycleBusiness, []string{"id", "version", "source_connection_id", "party_kind", "identity_namespace", "external_id", "created_at"}},
	{"source_observation_parties", LifecycleBusiness, []string{"id", "version", "source_connection_id", "source_observation_id", "source_party_id", "evidence_reference_id", "role", "display_name_snapshot", "homepage_url_snapshot", "created_at"}},
	{"contents", LifecycleBusiness, []string{"id", "source_connection_id", "external_id", "dedupe_key", "dedupe_reason", "dedupe_version", "view_count", "like_count", "comment_count", "share_count", "deleted_at"}},
	{"content_assets", LifecycleBusiness, []string{"id", "content_id", "object_key", "object_status"}},
	{"documents", LifecycleBusiness, []string{"id", "version", "source_connection_id", "document_key", "current_document_version_id", "document_state"}},
	{"document_versions", LifecycleBusiness, []string{"id", "version", "document_id", "source_observation_id", "revision_no", "version_key", "quality_score", "content_sha256", "extractor_profile_version", "extractor_profile_sha256", "display_private_rights_decision_id", "lifecycle_state"}},
	{"document_identity_keys", LifecycleBusiness, []string{"id", "version", "source_connection_id", "document_id", "identity_kind", "identity_value"}},
	{"derived_artifacts", LifecycleOperational, []string{"id", "source_connection_id", "document_version_id", "store_derived_rights_decision_id", "retain_rights_decision_id", "artifact_type", "transformer_profile_sha256", "vault_relative_path", "sha256", "anchor_normalization_version", "anchor_map_profile_version", "anchor_plaintext_sha256", "anchor_markdown_sha256", "anchor_map_sha256", "retention_until", "lifecycle_state", "active"}},
	{"derived_artifact_deletion_audits", LifecycleOperational, []string{"id", "derived_artifact_id", "source_connection_id", "retention_policy_id", "retention_policy_version", "attempt_no", "event_type", "vault_relative_path", "sha256", "size_bytes", "reason_code", "already_missing", "occurred_at", "created_at"}},
	{"document_anchor_blocks", LifecycleOperational, []string{"id", "derived_artifact_id", "anchor_map_sha256", "block_ordinal", "plaintext_utf8_byte_start", "plaintext_utf8_byte_end", "markdown_utf8_byte_start", "markdown_utf8_byte_end", "markdown_anchor", "created_at"}},
	{"document_text_quote_selectors", LifecycleOperational, []string{"id", "version", "source_connection_id", "document_version_id", "plaintext_artifact_id", "markdown_artifact_id", "quote_rights_decision_id", "retain_rights_decision_id", "exact_quote", "prefix", "suffix", "utf8_byte_start", "utf8_byte_end", "quote_sha256", "plaintext_sha256", "normalization_version", "selector_version", "anchor_map_sha256", "markdown_anchor", "retention_until", "created_at"}},
	{"content_fingerprints", LifecycleOperational, []string{"id", "version", "source_connection_id", "document_version_id", "derived_artifact_id", "store_derived_rights_decision_id", "retain_rights_decision_id", "profile_version", "normalized_content_sha256", "simhash_hex", "minhash", "retention_until", "lifecycle_state", "tombstoned_at", "purge_reason", "created_at"}},
	{"content_families", LifecycleBusiness, []string{"id", "version", "root_document_version_id", "lineage_profile_version", "status", "created_at", "updated_at"}},
	{"content_lineage_decisions", LifecycleOperational, []string{"id", "version", "document_version_id", "fingerprint_id", "family_id", "result_family_version", "candidate_root_document_version_id", "action", "relation", "hamming_distance", "minhash_similarity", "decision_profile_version", "reason_codes", "decision_origin", "decided_by_user_id", "idempotency_key", "command_fingerprint", "created_at"}},
	{"content_family_members", LifecycleBusiness, []string{"id", "version", "family_id", "document_version_id", "fingerprint_id", "lineage_decision_id", "lineage_profile_version", "relation", "parent_document_version_id", "active", "created_at", "retired_at"}},
	{"content_lineage_feedbacks", LifecycleBusiness, []string{"id", "version", "lineage_decision_id", "result_lineage_decision_id", "document_version_id", "original_family_id", "result_family_id", "original_relation", "result_relation", "original_parent_document_version_id", "result_parent_document_version_id", "result_family_version", "actor_user_id", "feedback_type", "relation_override", "reason_code", "note", "idempotency_key", "command_fingerprint", "created_at"}},
	{"micro_events", LifecycleBusiness, []string{"id", "version", "event_key", "status", "primary_subject_key", "primary_action_key", "location_keys", "identifier_keys", "event_started_at", "event_ended_at", "clustering_profile_version", "merged_into_micro_event_id", "created_at", "updated_at"}},
	{"micro_event_membership_decisions", LifecycleOperational, []string{"id", "version", "content_family_id", "document_match_decision_id", "monitor_id", "monitor_version_id", "candidate_micro_event_id", "resulting_micro_event_id", "result_event_version", "action", "same_event_score", "leading_margin", "sparse_similarity", "dense_similarity", "entity_overlap", "action_overlap", "location_consistency", "identifier_consistency", "time_similarity", "lineage_relation", "hard_conflict_reasons", "clustering_profile_version", "reason_codes", "decision_origin", "actor_user_id", "idempotency_key", "command_fingerprint", "created_at"}},
	{"micro_event_members", LifecycleBusiness, []string{"id", "version", "micro_event_id", "content_family_id", "membership_decision_id", "clustering_profile_version", "active", "created_at", "retired_at"}},
	{"micro_event_feedbacks", LifecycleBusiness, []string{"id", "version", "membership_decision_id", "micro_event_id", "original_event_version", "content_family_id", "actor_user_id", "feedback_type", "target_micro_event_id", "target_event_version", "result_micro_event_id", "result_event_version", "result_event_status", "result_target_micro_event_id", "result_target_event_version", "result_target_event_status", "result_membership_decision_id", "result_member_version", "governance_profile_version", "reason_code", "note", "idempotency_key", "command_fingerprint", "created_at"}},
	{"storylines", LifecycleBusiness, []string{"id", "version", "storyline_key", "title", "summary", "status", "relation_profile_version", "created_at", "updated_at"}},
	{"storyline_events", LifecycleBusiness, []string{"id", "version", "storyline_id", "micro_event_id", "source_micro_event_version", "result_storyline_version", "relation_type", "relation_score", "relation_profile_version", "reason_codes", "decision_origin", "actor_user_id", "storyline_key_snapshot", "storyline_title_snapshot", "storyline_status_snapshot", "idempotency_key", "command_fingerprint", "active", "created_at", "retired_at"}},
	{"event_heat_profiles", LifecycleBusiness, []string{"id", "version", "profile_version", "status", "lineage_weight", "velocity_weight", "acceleration_weight", "coverage_weight", "engagement_weight", "recency_weight", "evaluation_run_id", "activated_by_user_id", "activated_at", "rolled_back_by_user_id", "rolled_back_at", "created_at"}},
	{"micro_event_heat_snapshots", LifecycleOperational, []string{"id", "version", "micro_event_id", "micro_event_version", "heat_profile_id", "window_started_at", "window_ended_at", "independent_lineage_root_count", "velocity", "acceleration", "coverage", "normalized_engagement", "recency", "available_weight", "heat_score", "reason_codes", "calculated_at"}},
	{"claims", LifecycleBusiness, []string{"id", "version", "micro_event_id", "micro_event_version", "claim_hash", "subject", "predicate", "object", "qualifiers", "created_at"}},
	{"claim_evidence_versions", LifecycleOperational, []string{"id", "version", "claim_id", "document_version_id", "text_quote_selector_id", "content_family_id", "lineage_root_document_version_id", "relation", "quote_sha256", "plaintext_sha256", "selector_version", "source_record_url_snapshot", "canonical_url_snapshot", "publisher_party_id", "publisher_name_snapshot", "content_origin_party_id", "content_origin_name_snapshot", "published_at_snapshot", "captured_at_snapshot", "model_run_id", "model_relation_score", "extraction_schema_version", "decision_origin", "actor_user_id", "idempotency_key", "command_fingerprint", "retention_until", "created_at"}},
	{"evidence_state_profiles", LifecycleBusiness, []string{"id", "version", "algorithm_version", "status", "activated_by_user_id", "activated_at", "rolled_back_by_user_id", "rolled_back_at", "created_at"}},
	{"evidence_state_snapshots", LifecycleOperational, []string{"id", "version", "micro_event_id", "micro_event_version", "evidence_state_profile_id", "algorithm_version", "evidence_set_hash", "evidence_state", "independent_origin_count", "reason_codes", "calculated_at", "created_at"}},
	{"evidence_state_snapshot_items", LifecycleOperational, []string{"id", "evidence_state_snapshot_id", "claim_evidence_version_id", "ordinal", "created_at"}},
	{"claim_evidence_feedbacks", LifecycleBusiness, []string{"id", "version", "claim_id", "original_claim_evidence_version_id", "result_claim_evidence_version_id", "target_document_version_id", "original_text_quote_selector_id", "result_text_quote_selector_id", "original_relation", "result_relation", "actor_user_id", "expected_claim_version", "reason_code", "note", "idempotency_key", "command_fingerprint", "created_at"}},
	{"micro_event_summaries", LifecycleBusiness, []string{"id", "version", "micro_event_id", "micro_event_version", "summary_profile_version", "idempotency_key", "command_fingerprint", "created_at"}},
	{"micro_event_summary_sentences", LifecycleBusiness, []string{"id", "version", "micro_event_summary_id", "ordinal", "sentence", "editorial_note", "decision_origin", "model_run_id", "actor_user_id", "created_at"}},
	{"micro_event_summary_sentence_evidences", LifecycleOperational, []string{"id", "summary_sentence_id", "claim_evidence_version_id", "ordinal", "created_at"}},
	{"micro_event_updates", LifecycleOperational, []string{"id", "version", "micro_event_id", "micro_event_version", "window_ended_at", "window_profile", "heat_profile_id", "heat_profile_version", "evidence_state_profile_id", "evidence_state_algorithm_version", "heat_snapshot_1h_id", "heat_snapshot_6h_id", "heat_snapshot_24h_id", "evidence_state_snapshot_id", "heat_score", "evidence_state", "independent_origin_count", "reason_codes", "refresh_key", "created_at"}},
	{"micro_event_alert_evaluations", LifecycleOperational, []string{"id", "version", "micro_event_update_id", "monitor_id", "monitor_config_version_id", "heat_score", "heat_threshold", "result", "notification_outbox_event_id", "idempotency_key", "evaluated_at", "created_at"}},
	{"notification_outbox_events", LifecycleOperational, []string{"id", "version", "event_type", "resource_type", "resource_id", "resource_version", "monitor_id", "occurred_at", "title", "summary", "resource_status", "deep_link", "dedupe_key", "created_at"}},
	{"user_notifications", LifecycleOperational, []string{"id", "version", "outbox_event_id", "user_id", "monitor_id", "event_type", "resource_type", "resource_id", "resource_version", "occurred_at", "title", "summary", "resource_status", "deep_link", "created_at"}},
	{"notification_read_receipts", LifecycleOperational, []string{"id", "version", "user_id", "read_through_id", "previous_receipt_id", "recorded_at"}},
	{"notification_delivery_attempts", LifecycleOperational, []string{"id", "version", "user_notification_id", "channel", "delivery_target_key", "attempt_no", "status", "dispatch_key", "fencing_generation", "provider_supports_idempotency", "provider_supports_receipt_lookup", "provider_message_id", "response_code", "error_code", "attempted_at", "created_at"}},
	{"notification_delivery_claims", LifecycleOperational, []string{"user_notification_id", "channel", "delivery_target_key", "claim_token", "fencing_generation", "dispatch_key", "provider_supports_idempotency", "provider_supports_receipt_lookup", "dispatch_started_at", "claimed_at", "lease_until"}},
	{"backup_runs", LifecycleOperational, []string{"id", "version", "run_sha256", "manifest_sha256", "git_revision", "status", "recovery_point_at", "started_at", "completed_at", "failure_code", "asset_count", "created_at"}},
	{"backup_retention_dispositions", LifecycleOperational, []string{"id", "version", "disposition_sha256", "manifest_sha256", "backup_run_id", "backup_run_sha256", "deletion_evidence_sha256", "status", "reason_code", "operator_record_id", "reviewer_record_id", "disposed_at", "created_at"}},
	{"projection_recovery_runs", LifecycleOperational, []string{"id", "version", "run_sha256", "status", "operator_record_id", "reviewer_record_id", "backup_evidence_sha256", "rehearsal_evidence_sha256", "notification_facts_before_sha256", "notification_facts_after_sha256", "vault_manual_before_sha256", "vault_manual_after_sha256", "notification_outbox_count", "user_notification_count", "read_receipt_count", "delivery_attempt_count", "max_user_notification_id", "max_read_receipt_id", "max_delivery_attempt_id", "removed_delivery_claim_count", "scheduled_vault_recovery_count", "scheduled_search_rebuild_count", "preserved_started_claim_count", "preserved_unknown_attempt_count", "created_at"}},
	{"web_push_subscriptions", LifecycleOperational, []string{"id", "version", "user_id", "endpoint_sha256", "endpoint_ciphertext", "p256dh_ciphertext", "auth_ciphertext", "encryption_key_version", "device_label", "timezone", "quiet_start", "quiet_end", "ttl_seconds", "status", "expiration_reason", "last_success_at", "last_failure_at", "idempotency_key", "command_fingerprint", "created_at", "updated_at"}},
	{"web_push_subscription_monitors", LifecycleOperational, []string{"id", "subscription_id", "monitor_id", "created_at"}},
	{"document_version_search_indexes", LifecycleOperational, []string{"id", "version", "document_version_id", "source_connection_id", "derived_artifact_id", "store_derived_rights_decision_id", "retain_rights_decision_id", "normalization_profile_version", "normalized_text_sha256", "title_search_vector", "body_search_vector", "title_trigrams", "body_trigrams", "entity_keys", "action_keys", "location_keys", "region_keys", "lifecycle_state", "tombstoned_at", "purge_reason", "retention_until", "indexed_at", "created_at"}},
	{"document_version_embeddings", LifecycleOperational, []string{"id", "document_version_id", "source_connection_id", "embed_local_rights_decision_id", "retain_rights_decision_id", "model_profile_id", "model_profile_version", "model_version", "normalized_text_sha256", "embedding", "ai_run_id", "retention_until", "lifecycle_state", "tombstoned_at", "purge_reason", "created_at"}},
	{"monitor_matches", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "input_hash", "scoring_version", "final_score", "decision", "decision_origin", "embedding_model_profile_id", "embedding_model_profile_version", "embedding_model_version", "review_ai_run_id"}},
	{"monitor_match_feedbacks", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "content_id", "monitor_match_id", "actor_user_id", "feedback_type"}},
	{"monitor_feedback_suggestions", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "suggestion_type", "value", "support_count", "status", "reviewed_by_user_id"}},
	{"events", LifecycleBusiness, []string{"id", "event_key", "lifecycle_status", "heat_score", "trend_score", "trend_status", "heat_window_hours", "heat_version", "heat_reason_codes", "metric_capability_profile_set_hash", "heat_calculated_at", "deleted_at"}},
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
	{"reports", LifecycleBusiness, []string{"id", "version", "report_type", "period_start", "input_snapshot_hash", "status", "version_no", "submitted_at", "submitted_by", "reviewed_at", "reviewed_by", "review_reason", "deleted_at"}},
	{"report_revision_transitions", LifecycleOperational, []string{"id", "report_id", "from_status", "to_status", "expected_resource_version", "result_resource_version", "actor_user_id", "reason_code", "created_at"}},
	{"report_items", LifecycleBusiness, []string{"id", "report_id", "event_id", "micro_event_id", "micro_event_version", "micro_event_update_id", "micro_event_summary_id", "rank"}},
	{"report_item_sentences", LifecycleBusiness, []string{"id", "version", "report_item_id", "source_summary_sentence_id", "ordinal", "sentence", "editorial_note", "decision_origin", "model_run_id", "actor_user_id", "created_at"}},
	{"report_item_sentence_evidences", LifecycleOperational, []string{"id", "report_item_sentence_id", "claim_evidence_version_id", "ordinal", "created_at"}},
	{"report_subscriptions", LifecycleBusiness, []string{"id", "user_id", "channel", "deleted_at"}},
	{"ai_model_profiles", LifecycleBusiness, []string{"id", "version", "name", "task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions", "timeout_seconds", "max_attempts", "max_cost", "daily_budget", "fallback_priority", "enabled", "deleted_at"}},
	{"retention_policies", LifecycleBusiness, []string{"id", "data_class", "retention_days", "action"}},
	{"retention_runs", LifecycleOperational, []string{"id", "version", "retention_policy_id", "retention_policy_version", "data_class", "cutoff", "batch_size", "candidate_count", "has_more", "candidate_hash", "status", "requested_by_user_id", "approved_by_user_id", "approved_at", "executed_by_user_id", "executed_at", "affected", "failure_code", "created_at", "updated_at"}},
	{"retention_run_items", LifecycleOperational, []string{"id", "retention_run_id", "ordinal", "candidate_id", "created_at"}},
	{"quota_usage_ledgers", LifecycleOperational, []string{"id", "dimension", "subject_type", "subject_id", "window_start", "window_end", "used", "updated_at"}},
	{"source_request_usage_ledgers", LifecycleOperational, []string{"id", "source_connection_id", "resource_profile_version", "budget_day", "used", "updated_at"}},
	{"auth_sessions", LifecycleOperational, []string{"id", "user_id", "family_id", "absolute_expires_at", "revoked_at"}},
	{"auth_refresh_tokens", LifecycleOperational, []string{"id", "session_id", "token_hash", "expires_at", "used_at", "revoked_at"}},
	{"source_checkpoints", LifecycleOperational, []string{"id", "monitor_source_id", "last_successful_run_id", "last_fetched_at", "next_poll_at"}},
	{"collection_runs", LifecycleOperational, []string{"id", "source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "retry_after", "page_count", "window_start", "window_end", "status", "updated_at"}},
	{"collection_run_targets", LifecycleOperational, []string{"id", "collection_run_id", "monitor_source_id", "monitor_config_version_id", "target_status", "updated_at"}},
	{"collection_run_items", LifecycleOperational, []string{"id", "run_id", "source_connection_id", "source_code", "external_id", "content_type", "captured_item_version", "captured_item", "payload_hash", "raw_payload_disposition", "content_id", "ingestion_status", "ingestion_error_code", "outcome", "observed_at"}},
	{"collection_run_target_items", LifecycleOperational, []string{"id", "collection_run_id", "collection_run_target_id", "collection_run_item_id", "outcome"}},
	{"content_metric_snapshots", LifecycleOperational, []string{"id", "content_id", "captured_at", "view_count", "like_count", "comment_count", "share_count"}},
	{"event_metric_snapshots", LifecycleOperational, []string{"id", "event_id", "captured_at", "window_hours", "heat_score", "trend_score", "trend_status", "heat_version", "evidence_set_hash", "capability_profile_set_hash"}},
	{"event_updates", LifecycleOperational, []string{"id", "version", "event_id", "sequence_no", "kind", "summary", "observed_at", "reason_codes", "before_state", "after_state", "evidence_set_hash", "idempotency_key", "created_at"}},
	{"alert_threads", LifecycleBusiness, []string{"id", "version", "monitor_id", "monitor_config_version_id", "monitor_revision", "monitor_config_hash", "event_id", "trigger_type", "policy_version", "state", "severity", "event_threshold_snapshot", "alert_min_heat_snapshot", "alert_min_momentum_snapshot", "alert_min_breadth_snapshot", "alert_warning_threshold_snapshot", "alert_critical_threshold_snapshot", "alert_cooldown_minutes_snapshot", "title_snapshot", "reason_snapshot", "first_triggered_at", "last_triggered_at", "occurrence_count", "cooldown_until", "acknowledged_at", "acknowledged_by_user_id", "resolved_at", "resolved_by_user_id", "suppressed_at", "suppressed_by_user_id", "created_at", "updated_at"}},
	{"alert_occurrences", LifecycleOperational, []string{"id", "alert_thread_id", "event_update_id", "severity", "final_score_snapshot", "threshold_snapshot", "heat_score_snapshot", "momentum_score_snapshot", "breadth_score_snapshot", "reason_codes", "fingerprint", "triggered_at", "created_at"}},
	{"alert_email_deliveries", LifecycleOperational, []string{"id", "occurrence_id", "idempotency_key", "severity", "status", "next_attempt_at", "succeeded_at"}},
	{"alert_email_attempts", LifecycleOperational, []string{"id", "delivery_id", "attempt_no", "status"}},
	{"alert_state_audits", LifecycleOperational, []string{"id", "alert_thread_id", "actor_type", "actor_user_id", "from_state", "to_state", "expected_version", "reason_code", "created_at"}},
	{"notification_events", LifecycleOperational, []string{"id", "event_type", "resource_type", "resource_id", "audience_role", "occurred_at", "payload", "dedupe_key", "created_at"}},
	{"ai_runs", LifecycleOperational, []string{"id", "owning_job_id", "workspace_key", "skill_id", "task_type", "target_type", "target_id", "target_version", "runtime_version", "model_profile_id", "model_profile_version", "model_version", "prompt_version", "input_schema_version", "schema_version", "parameters_version", "input_hash", "evidence_set_hash", "reuse_key", "attempt", "max_attempts", "repair_attempted", "retry_after", "error_code", "budget_day", "reserved_cost", "lease_expires_at", "status"}},
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
	{"audit_logs", LifecycleOperational, []string{"id", "action", "resource_type", "idempotency_key", "command_fingerprint", "result"}},
	{"evidence_lineage_migration_runs", LifecycleOperational, []string{"id", "version", "phase", "status", "execution_mode", "operator_id", "reviewer_id", "binary_sha256", "schema_sha256", "configuration_sha256", "backup_evidence_sha256", "rehearsal_evidence_sha256", "batch_size", "last_legacy_resource_id", "examined_count", "reused_count", "created_count", "skipped_count", "blocked_count", "failed_count", "resume_count", "failure_code", "started_at", "completed_at", "updated_at"}},
	{"evidence_lineage_migration_items", LifecycleOperational, []string{"id", "run_id", "phase", "legacy_resource_type", "legacy_resource_id", "input_sha256", "disposition", "target_resource_type", "target_resource_id", "reason_code", "attempt", "created_at"}},
	{"evidence_lineage_reconciliation_runs", LifecycleOperational, []string{"id", "version", "scope", "status", "execution_mode", "operator_id", "reviewer_id", "binary_sha256", "schema_sha256", "configuration_sha256", "backup_evidence_sha256", "rehearsal_evidence_sha256", "batch_size", "grace_period_hours", "backup_disposition_count", "last_asset_id", "examined_count", "healthy_count", "finding_count", "repaired_count", "failed_count", "resume_count", "failure_code", "started_at", "completed_at", "updated_at"}},
	{"evidence_lineage_reconciliation_items", LifecycleOperational, []string{"id", "run_id", "scope", "asset_type", "asset_id", "asset_key_sha256", "source_connection_id", "finding", "expected_sha256", "observed_sha256", "lifecycle_before", "lifecycle_after", "repair_action", "reason_code", "created_at"}},
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
		if spec.Table == "monitor_intent_drafts" {
			persistence.VersionColumn = "resource_version"
		}
		if spec.Table == "monitor_intent_preview_results" {
			persistence.KeyColumn = "run_id"
			persistence.AllowedSort = []string{"run_id"}
			persistence.CursorFields = []string{"run_id"}
		}
		switch {
		case spec.Table == "monitor_config_versions" || spec.Table == "monitor_rules" || spec.Table == "monitor_sources" ||
			spec.Table == "source_rights_policies" || spec.Table == "source_observations" ||
			spec.Table == "documents" || spec.Table == "document_versions" || spec.Table == "document_identity_keys" ||
			strings.HasPrefix(spec.Table, "monitor_intent_") || strings.HasPrefix(spec.Table, "monitor_compiled_"):
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
