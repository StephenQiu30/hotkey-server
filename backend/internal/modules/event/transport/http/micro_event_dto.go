package http

import "time"

type MicroEventV2Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type StorylineResponseDTO struct {
	ID                     int64  `json:"id"`
	Version                int64  `json:"version"`
	StorylineKey           string `json:"storyline_key"`
	Title                  string `json:"title"`
	Summary                string `json:"summary"`
	Status                 string `json:"status"`
	RelationProfileVersion string `json:"relation_profile_version"`
}
type EventHeatV2ResponseDTO struct {
	ID                          int64     `json:"id"`
	MicroEventVersion           int64     `json:"micro_event_version"`
	WindowStartedAt             time.Time `json:"window_started_at"`
	WindowEndedAt               time.Time `json:"window_ended_at"`
	IndependentLineageRootCount int       `json:"independent_lineage_root_count"`
	Velocity                    float64   `json:"velocity"`
	Acceleration                float64   `json:"acceleration"`
	Coverage                    float64   `json:"coverage"`
	NormalizedEngagement        *float64  `json:"normalized_engagement"`
	Recency                     float64   `json:"recency"`
	HeatScore                   float64   `json:"heat_score"`
	ReasonCodes                 []string  `json:"reason_codes"`
}
type EvidenceStateResponseDTO struct {
	ID                     int64     `json:"id"`
	EventVersion           int64     `json:"event_version"`
	AlgorithmVersion       string    `json:"algorithm_version"`
	State                  string    `json:"state"`
	IndependentOriginCount int       `json:"independent_origin_count"`
	ReasonCodes            []string  `json:"reason_codes"`
	CalculatedAt           time.Time `json:"calculated_at"`
}
type EvidenceSummarySentenceResponseDTO struct {
	ID                      int64   `json:"id"`
	Ordinal                 int     `json:"ordinal"`
	Text                    string  `json:"text"`
	EditorialNote           bool    `json:"editorial_note"`
	ClaimEvidenceVersionIDs []int64 `json:"claim_evidence_version_ids"`
	DecisionOrigin          string  `json:"decision_origin"`
}
type EvidenceSummaryResponseDTO struct {
	ID                    int64                                `json:"id"`
	EventVersion          int64                                `json:"event_version"`
	SummaryProfileVersion string                               `json:"summary_profile_version"`
	Sentences             []EvidenceSummarySentenceResponseDTO `json:"sentences"`
	CreatedAt             time.Time                            `json:"created_at"`
}
type MicroEventResponseDTO struct {
	ID                       int64                       `json:"id"`
	Version                  int64                       `json:"version"`
	EventKey                 string                      `json:"event_key"`
	Status                   string                      `json:"status"`
	PrimarySubjectKey        string                      `json:"primary_subject_key"`
	PrimaryActionKey         string                      `json:"primary_action_key"`
	LocationKeys             []string                    `json:"location_keys"`
	IdentifierKeys           []string                    `json:"identifier_keys"`
	EventStartedAt           time.Time                   `json:"event_started_at"`
	EventEndedAt             *time.Time                  `json:"event_ended_at,omitempty"`
	ClusteringProfileVersion string                      `json:"clustering_profile_version"`
	Storyline                *StorylineResponseDTO       `json:"storyline,omitempty"`
	LatestHeat               *EventHeatV2ResponseDTO     `json:"latest_heat,omitempty"`
	EvidenceState            *EvidenceStateResponseDTO   `json:"evidence_state,omitempty"`
	EvidenceSummary          *EvidenceSummaryResponseDTO `json:"evidence_summary,omitempty"`
	ContentFamilyCount       int                         `json:"content_family_count"`
	DocumentCount            int                         `json:"document_count"`
}
type MicroEventPageResponseDTO struct {
	Items        []MicroEventResponseDTO `json:"items"`
	NextCursorID int64                   `json:"next_cursor_id,omitempty"`
}

type ClaimEvidenceResponseDTO struct {
	ID                         int64      `json:"id"`
	Version                    int64      `json:"version"`
	ClaimID                    int64      `json:"claim_id"`
	DocumentVersionID          int64      `json:"document_version_id"`
	TextQuoteSelectorID        int64      `json:"text_quote_selector_id"`
	ContentFamilyID            int64      `json:"content_family_id"`
	LineageRootID              int64      `json:"lineage_root_document_version_id"`
	LineageDecisionID          *int64     `json:"lineage_decision_id,omitempty"`
	ContentFamilyMemberVersion *int64     `json:"content_family_member_version,omitempty"`
	Subject                    string     `json:"subject"`
	Predicate                  string     `json:"predicate"`
	Object                     string     `json:"object"`
	Relation                   string     `json:"relation"`
	Availability               string     `json:"availability"`
	ExactQuote                 *string    `json:"exact_quote,omitempty"`
	Prefix                     *string    `json:"prefix,omitempty"`
	Suffix                     *string    `json:"suffix,omitempty"`
	UTF8ByteStart              *int64     `json:"utf8_byte_start,omitempty"`
	UTF8ByteEnd                *int64     `json:"utf8_byte_end,omitempty"`
	QuoteSHA256                *string    `json:"quote_sha256,omitempty"`
	PlaintextSHA256            *string    `json:"plaintext_sha256,omitempty"`
	SelectorVersion            *string    `json:"selector_version,omitempty"`
	MarkdownAnchor             *string    `json:"markdown_anchor,omitempty"`
	SourceRecordURL            *string    `json:"source_record_url,omitempty"`
	CanonicalURL               *string    `json:"canonical_url,omitempty"`
	PublisherName              *string    `json:"publisher,omitempty"`
	ContentOriginName          *string    `json:"content_origin,omitempty"`
	PublishedAt                *time.Time `json:"published_at,omitempty"`
	CapturedAt                 time.Time  `json:"captured_at"`
	ExtractionSchemaVersion    string     `json:"extraction_schema_version"`
	DecisionOrigin             string     `json:"decision_origin"`
	CreatedAt                  time.Time  `json:"created_at"`
}
type MicroEventEvidencePageResponseDTO struct {
	Items        []ClaimEvidenceResponseDTO `json:"items"`
	NextCursorID int64                      `json:"next_cursor_id,omitempty"`
}

type ClaimQualifierRequestDTO struct {
	Key   string `json:"key" binding:"required,max=64"`
	Value string `json:"value" binding:"required,max=512"`
}
type RecordClaimEvidenceRequestDTO struct {
	ExpectedEventVersion int64                      `json:"expected_event_version" binding:"required"`
	DocumentVersionID    int64                      `json:"document_version_id" binding:"required"`
	TextQuoteSelectorID  int64                      `json:"text_quote_selector_id" binding:"required"`
	Subject              string                     `json:"subject" binding:"required,max=512"`
	Predicate            string                     `json:"predicate" binding:"required,max=256"`
	Object               string                     `json:"object" binding:"required,max=2000"`
	Qualifiers           []ClaimQualifierRequestDTO `json:"qualifiers"`
	Relation             string                     `json:"relation" binding:"required"`
}
type CorrectClaimEvidenceRequestDTO struct {
	ExpectedClaimVersion      int64  `json:"expected_claim_version" binding:"required"`
	ResultTextQuoteSelectorID int64  `json:"result_text_quote_selector_id" binding:"required"`
	ResultRelation            string `json:"result_relation" binding:"required"`
	ReasonCode                string `json:"reason_code" binding:"required,max=64"`
	Note                      string `json:"note" binding:"max=1000"`
}
type MicroEventGovernanceRequestDTO struct {
	ExpectedEventVersion       int64  `json:"expected_event_version" binding:"required"`
	Action                     string `json:"action" binding:"required"`
	MembershipDecisionID       int64  `json:"membership_decision_id"`
	ContentFamilyID            int64  `json:"content_family_id"`
	ExpectedMemberVersion      int64  `json:"expected_member_version"`
	TargetMicroEventID         int64  `json:"target_micro_event_id"`
	ExpectedTargetEventVersion int64  `json:"expected_target_event_version"`
	ReasonCode                 string `json:"reason_code" binding:"required,max=64"`
	Note                       string `json:"note" binding:"max=1000"`
}
type ClaimEvidenceMutationResponseDTO struct {
	ClaimID         int64                     `json:"claim_id"`
	ClaimVersion    int64                     `json:"claim_version"`
	EvidenceID      int64                     `json:"evidence_id"`
	EvidenceVersion int64                     `json:"evidence_version"`
	EvidenceState   *EvidenceStateResponseDTO `json:"evidence_state,omitempty"`
}
type ClaimEvidenceCorrectionResponseDTO struct {
	FeedbackID      int64                     `json:"feedback_id"`
	EvidenceID      int64                     `json:"evidence_id"`
	EvidenceVersion int64                     `json:"evidence_version"`
	EvidenceState   *EvidenceStateResponseDTO `json:"evidence_state,omitempty"`
}
type MicroEventGovernanceResponseDTO struct {
	FeedbackID  int64                          `json:"feedback_id"`
	SourceEvent MicroEventGovernanceResultDTO  `json:"source_event"`
	TargetEvent *MicroEventGovernanceResultDTO `json:"target_event,omitempty"`
}
type MicroEventGovernanceResultDTO struct {
	ID      int64  `json:"id"`
	Version int64  `json:"version"`
	Status  string `json:"status"`
}
