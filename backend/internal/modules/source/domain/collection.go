package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func CollectionClaimKey(sourceConnectionID int64, querySignature string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("hotkey.collection_claim:%d:%s", sourceConnectionID, querySignature)))
	return hex.EncodeToString(digest[:])
}

const (
	// CapturedItemVersionV1 is the legacy PLAN-006 envelope. Its integer
	// metrics cannot distinguish an omitted value from an explicit zero.
	CapturedItemVersionV1 = "v1"
	// CapturedItemVersionV2 preserves metric presence with nullable values.
	CapturedItemVersionV2 = "v2"
)

type RawPayloadDisposition string

const (
	RawPayloadDiscarded        RawPayloadDisposition = "discarded"
	RawPayloadCapturedItemOnly RawPayloadDisposition = "captured_item_only"
)

func (disposition RawPayloadDisposition) Valid() bool {
	return disposition == RawPayloadDiscarded || disposition == RawPayloadCapturedItemOnly
}

type EvidenceCompleteness string

const (
	// EvidenceCompletenessUnknown preserves compatibility for connectors that
	// have not declared whether an upstream body is complete or excerpted.
	EvidenceCompletenessUnknown      EvidenceCompleteness = ""
	EvidenceCompletenessMetadataOnly EvidenceCompleteness = "metadata_only"
	EvidenceCompletenessSummaryOnly  EvidenceCompleteness = "summary_only"
	EvidenceCompletenessFullBody     EvidenceCompleteness = "full_body"
	MaxSourceAttachments                                  = 32
)

func (completeness EvidenceCompleteness) Valid() bool {
	return completeness == EvidenceCompletenessUnknown || completeness == EvidenceCompletenessMetadataOnly || completeness == EvidenceCompletenessSummaryOnly || completeness == EvidenceCompletenessFullBody
}

// SourceAttachment is metadata declared by a Feed. Connectors never download
// the referenced bytes; binary capture remains an ingestion concern.
type SourceAttachment struct {
	URL       string `json:"url"`
	MIMEType  string `json:"mime_type,omitempty"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
}

// FetchRequest is the protocol-neutral request for one shared collection run.
// It intentionally carries no Monitor target identity: targets consume the
// resulting captured items after the one external request has completed.
type FetchRequest struct {
	CollectionRunID    int64
	SourceConnectionID int64
	QuerySignature     string
	Query              string
	Languages          []string
	Regions            []string
	WindowStart        time.Time
	WindowEnd          time.Time
	RequestCursor      string
	ETag               string
	LastModified       string
	Limit              int
}

func (request FetchRequest) Validate() error {
	if request.CollectionRunID <= 0 || request.SourceConnectionID <= 0 {
		return fmt.Errorf("collection run and source connection are required")
	}
	if !validSHA256(request.QuerySignature) {
		return fmt.Errorf("query signature must be a SHA-256 hex value")
	}
	if request.WindowStart.IsZero() || request.WindowEnd.IsZero() || !request.WindowEnd.After(request.WindowStart) {
		return fmt.Errorf("collection window is invalid")
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return fmt.Errorf("collection fetch limit must be 1-1000")
	}
	return nil
}

// SourceItem is the stable Connector output. It refers to one response
// snapshot and an item locator, but never owns or copies raw upstream bytes.
type SourceItem struct {
	SourceCode           string
	ExternalID           string
	ParentExternalID     string
	ContentType          string
	Title                string
	Body                 string
	Language             string
	URL                  string
	DiscussionURL        string
	Author               string
	PublishedAt          *time.Time
	ObservedAt           time.Time
	EvidenceCompleteness EvidenceCompleteness
	Attachments          []SourceAttachment
	Metrics              SourceMetrics
	Parties              []SourcePartyAssertion
	SnapshotKey          string
	ItemLocator          string
	EvidenceReferences   []EvidenceReference
}

type SourceMetrics struct {
	ViewCount    *int64
	LikeCount    *int64
	CommentCount *int64
	ShareCount   *int64
}

// KnownMetric marks a metric as supplied by a source. A nil metric remains
// unknown; callers must not replace it with a pointer to zero.
func KnownMetric(value int64) *int64 { return &value }

func (metrics SourceMetrics) Validate() error {
	for _, metric := range []*int64{metrics.ViewCount, metrics.LikeCount, metrics.CommentCount, metrics.ShareCount} {
		if metric != nil && *metric < 0 {
			return fmt.Errorf("source metrics cannot be negative")
		}
	}
	return nil
}

func NormalizeSourceItem(item SourceItem) (SourceItem, error) {
	item.SourceCode = strings.ToLower(strings.TrimSpace(item.SourceCode))
	item.ExternalID = strings.TrimSpace(item.ExternalID)
	item.ParentExternalID = strings.TrimSpace(item.ParentExternalID)
	item.ContentType = strings.ToLower(strings.TrimSpace(item.ContentType))
	item.Title = strings.TrimSpace(item.Title)
	item.Body = strings.TrimSpace(item.Body)
	item.Language = strings.TrimSpace(item.Language)
	item.URL = strings.TrimSpace(item.URL)
	item.DiscussionURL = strings.TrimSpace(item.DiscussionURL)
	item.Author = strings.TrimSpace(item.Author)
	item.SnapshotKey = strings.ToLower(strings.TrimSpace(item.SnapshotKey))
	item.ItemLocator = strings.TrimSpace(item.ItemLocator)
	if (item.SnapshotKey == "") != (item.ItemLocator == "") {
		return SourceItem{}, fmt.Errorf("source item snapshot reference is incomplete")
	}
	if item.SnapshotKey != "" && (!validSHA256(item.SnapshotKey) || len(item.ItemLocator) > 1024 || strings.ContainsAny(item.ItemLocator, "\x00\r\n")) {
		return SourceItem{}, fmt.Errorf("source item snapshot reference is invalid")
	}
	if len(item.EvidenceReferences) > MaxEvidenceReferences {
		return SourceItem{}, fmt.Errorf("source item evidence reference count exceeds %d", MaxEvidenceReferences)
	}
	references := make([]EvidenceReference, 0, len(item.EvidenceReferences))
	seenReferences := make(map[string]struct{}, len(item.EvidenceReferences))
	for _, reference := range item.EvidenceReferences {
		reference.SnapshotKey = strings.ToLower(strings.TrimSpace(reference.SnapshotKey))
		if reference.Usage == "" {
			reference.Usage = EvidenceUsageDocumentSource
		}
		reference.LocatorValue = strings.TrimSpace(reference.LocatorValue)
		reference.SelectedPayloadSHA256 = strings.ToLower(strings.TrimSpace(reference.SelectedPayloadSHA256))
		reference.SelectorVersion = strings.TrimSpace(reference.SelectorVersion)
		if err := reference.Validate(); err != nil {
			return SourceItem{}, err
		}
		identity := reference.SnapshotKey + "\x00" + string(reference.LocatorType) + "\x00" + reference.LocatorValue
		if _, exists := seenReferences[identity]; exists {
			return SourceItem{}, fmt.Errorf("source item evidence reference is duplicated")
		}
		seenReferences[identity] = struct{}{}
		references = append(references, reference)
	}
	item.EvidenceReferences = references
	parties, err := NormalizeSourceParties(item.Parties)
	if err != nil {
		return SourceItem{}, err
	}
	item.Parties = parties
	if len(references) > 0 {
		if item.SnapshotKey == "" && item.ItemLocator == "" {
			item.SnapshotKey = references[0].SnapshotKey
			item.ItemLocator = references[0].LocatorValue
		} else if item.SnapshotKey != references[0].SnapshotKey || item.ItemLocator != references[0].LocatorValue {
			return SourceItem{}, fmt.Errorf("source item primary evidence reference is inconsistent")
		}
	}
	if !item.EvidenceCompleteness.Valid() {
		return SourceItem{}, fmt.Errorf("source item evidence completeness is invalid")
	}
	if item.Body == "" {
		if item.EvidenceCompleteness == EvidenceCompletenessSummaryOnly || item.EvidenceCompleteness == EvidenceCompletenessFullBody {
			return SourceItem{}, fmt.Errorf("source item evidence completeness requires a body")
		}
		item.EvidenceCompleteness = EvidenceCompletenessMetadataOnly
	} else if item.EvidenceCompleteness == EvidenceCompletenessMetadataOnly {
		return SourceItem{}, fmt.Errorf("metadata-only source item cannot include a body")
	}
	attachments, err := normalizeSourceAttachments(item.Attachments)
	if err != nil {
		return SourceItem{}, err
	}
	item.Attachments = attachments
	if item.SourceCode == "" || len(item.SourceCode) > 64 {
		return SourceItem{}, fmt.Errorf("source code must be 1-64 bytes")
	}
	if item.ExternalID == "" || len(item.ExternalID) > 512 {
		return SourceItem{}, fmt.Errorf("source item requires a stable external ID")
	}
	if len(item.ParentExternalID) > 512 || item.ParentExternalID == item.ExternalID {
		return SourceItem{}, fmt.Errorf("source item parent external ID is invalid")
	}
	normalizedURL, err := normalizeSourceItemURL(item.URL)
	if err != nil {
		return SourceItem{}, fmt.Errorf("source item canonical URL is invalid")
	}
	item.URL = normalizedURL
	discussionURL, err := normalizeSourceItemURL(item.DiscussionURL)
	if err != nil {
		return SourceItem{}, fmt.Errorf("source item discussion URL is invalid")
	}
	item.DiscussionURL = discussionURL
	if item.ContentType == "" || len(item.ContentType) > 32 {
		return SourceItem{}, fmt.Errorf("source item content type is invalid")
	}
	if item.ObservedAt.IsZero() {
		return SourceItem{}, fmt.Errorf("source item observed time is required")
	}
	if item.Language != "" {
		languages, err := normalizeLanguages([]string{item.Language}, 1, 1)
		if err != nil {
			return SourceItem{}, fmt.Errorf("normalize source item language: %w", err)
		}
		item.Language = languages[0]
	}
	if err := item.Metrics.Validate(); err != nil {
		return SourceItem{}, err
	}
	return item, nil
}

func normalizeSourceItemURL(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || len(value) > 2048 {
		return "", fmt.Errorf("invalid URL")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func normalizeSourceAttachments(attachments []SourceAttachment) ([]SourceAttachment, error) {
	if len(attachments) > MaxSourceAttachments {
		return nil, fmt.Errorf("source item attachment count exceeds %d", MaxSourceAttachments)
	}
	normalized := make([]SourceAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		attachment.URL = strings.TrimSpace(attachment.URL)
		attachment.MIMEType = strings.ToLower(strings.TrimSpace(attachment.MIMEType))
		parsed, err := url.Parse(attachment.URL)
		if err != nil || parsed == nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || len(attachment.URL) > 2048 || len(attachment.MIMEType) > 255 {
			return nil, fmt.Errorf("source attachment metadata is invalid")
		}
		if attachment.SizeBytes != nil {
			if *attachment.SizeBytes < 0 {
				return nil, fmt.Errorf("source attachment size cannot be negative")
			}
			size := *attachment.SizeBytes
			attachment.SizeBytes = &size
		}
		normalized = append(normalized, attachment)
	}
	return normalized, nil
}

// CapturePolicy centralizes the durable, versioned projection from a transient
// SourceItem. Source connectors therefore cannot select their own persistence
// shape or retain raw upstream bytes.
type CapturePolicy struct {
	Version               string
	RawPayloadDisposition RawPayloadDisposition
}

func (policy CapturePolicy) Validate() error {
	if policy.Version != CapturedItemVersionV2 {
		return fmt.Errorf("unsupported captured item version %q", policy.Version)
	}
	if policy.RawPayloadDisposition != RawPayloadDiscarded {
		return fmt.Errorf("new captured items must discard transient body and raw payload bytes")
	}
	return nil
}

type CapturedItem struct {
	Version               string
	SourceCode            string
	ExternalID            string
	ParentExternalID      string
	ContentType           string
	Title                 string
	Body                  string
	Language              string
	URL                   string
	Author                string
	PublishedAt           *time.Time
	ObservedAt            time.Time
	EvidenceCompleteness  EvidenceCompleteness
	Attachments           []SourceAttachment
	Metrics               SourceMetrics
	RawPayloadDisposition RawPayloadDisposition
	RawPayload            []byte
}

func (policy CapturePolicy) Capture(item SourceItem) (CapturedItem, error) {
	if err := policy.Validate(); err != nil {
		return CapturedItem{}, err
	}
	normalized, err := NormalizeSourceItem(item)
	if err != nil {
		return CapturedItem{}, err
	}
	captured := CapturedItem{
		Version: policy.Version, SourceCode: normalized.SourceCode, ExternalID: normalized.ExternalID,
		ParentExternalID: normalized.ParentExternalID, ContentType: normalized.ContentType, Title: normalized.Title, Language: normalized.Language,
		URL: normalized.URL, Author: normalized.Author, ObservedAt: normalized.ObservedAt,
		EvidenceCompleteness: normalized.EvidenceCompleteness, Attachments: normalized.Attachments,
		Metrics: normalized.Metrics, RawPayloadDisposition: policy.RawPayloadDisposition,
	}
	if normalized.PublishedAt != nil {
		publishedAt := normalized.PublishedAt.UTC()
		captured.PublishedAt = &publishedAt
	}
	// CapturedItem remains a metadata compatibility projection. Rights-aware
	// raw evidence and immutable DocumentVersion artifacts are the only new
	// body persistence path; legacy allow_body_storage is not authorization.
	captured.EvidenceCompleteness = EvidenceCompletenessMetadataOnly
	return captured, nil
}

type CollectionTerm struct {
	Value    string
	Excluded bool
}

const MaxCollectionQueryBytes = 2048

// CompileCollectionQuery is the single deterministic representation used by
// control-plane previews and collection workers. Current official connectors
// apply it as a local filter; they do not claim unsupported upstream syntax.
func CompileCollectionQuery(override string, terms []CollectionTerm) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		if len([]byte(override)) > MaxCollectionQueryBytes {
			return "", fmt.Errorf("collection query must be at most %d UTF-8 bytes", MaxCollectionQueryBytes)
		}
		return override, nil
	}
	normalized := make([]CollectionTerm, 0, len(terms))
	for _, term := range terms {
		term.Value = strings.Join(strings.Fields(term.Value), " ")
		if term.Value != "" {
			normalized = append(normalized, term)
		}
	}
	sort.Slice(normalized, func(left, right int) bool {
		if normalized[left].Excluded != normalized[right].Excluded {
			return !normalized[left].Excluded
		}
		return normalized[left].Value < normalized[right].Value
	})
	tokens := make([]string, 0, len(normalized))
	for _, term := range normalized {
		value := term.Value
		if strings.ContainsAny(value, " \t\r\n") {
			value = strconv.Quote(value)
		}
		if term.Excluded {
			value = "-" + value
		}
		tokens = append(tokens, value)
	}
	query := strings.Join(tokens, " ")
	if query == "" {
		return "", fmt.Errorf("collection query requires an effective term")
	}
	if len([]byte(query)) > MaxCollectionQueryBytes {
		return "", fmt.Errorf("collection query must be at most %d UTF-8 bytes", MaxCollectionQueryBytes)
	}
	return query, nil
}

// PublishedCollectionTarget is the Source-facing projection of one immutable
// published Monitor association. It is deliberately not a Monitor record.
type PublishedCollectionTarget struct {
	MonitorSourceID        int64
	MonitorConfigVersionID int64
	SourceConnectionID     int64
	QuerySignature         string
	QueryOverride          string
	Terms                  []CollectionTerm
	Languages              []string
	Regions                []string
	CollectionInterval     time.Duration
	Checkpoint             CollectionCheckpoint
}

func (target PublishedCollectionTarget) Validate() error {
	if target.MonitorSourceID <= 0 || target.MonitorConfigVersionID <= 0 || target.SourceConnectionID <= 0 {
		return fmt.Errorf("published collection target ownership is required")
	}
	if !validSHA256(target.QuerySignature) {
		return fmt.Errorf("published collection target requires a query signature")
	}
	if target.CollectionInterval < 5*time.Minute || target.CollectionInterval > 24*time.Hour || target.CollectionInterval%time.Minute != 0 {
		return fmt.Errorf("published collection interval is invalid")
	}
	if target.Checkpoint.MonitorSourceID != target.MonitorSourceID {
		return fmt.Errorf("checkpoint must belong to the published monitor source")
	}
	if target.Checkpoint.QueryHash != target.QuerySignature {
		return fmt.Errorf("checkpoint query hash must match the published query signature")
	}
	if err := target.Checkpoint.Validate(); err != nil {
		return err
	}
	for _, term := range target.Terms {
		if strings.TrimSpace(term.Value) == "" {
			return fmt.Errorf("collection term is required")
		}
	}
	return nil
}

type CollectionRequest struct {
	SourceConnectionID int64
	QuerySignature     string
	Query              string
	Languages          []string
	Regions            []string
	WindowStart        time.Time
	WindowEnd          time.Time
	ScheduledAt        time.Time
	TriggerType        CollectionTriggerType
	Targets            []PublishedCollectionTarget
}

type CollectionTriggerType string

const (
	CollectionTriggerSchedule  CollectionTriggerType = "schedule"
	CollectionTriggerManual    CollectionTriggerType = "manual"
	CollectionTriggerRetry     CollectionTriggerType = "retry"
	CollectionTriggerReconcile CollectionTriggerType = "reconcile"
)

func (trigger CollectionTriggerType) Valid() bool {
	return trigger == CollectionTriggerSchedule || trigger == CollectionTriggerManual || trigger == CollectionTriggerRetry || trigger == CollectionTriggerReconcile
}

func (request CollectionRequest) EffectiveTriggerType() CollectionTriggerType {
	if request.TriggerType == "" {
		return CollectionTriggerSchedule
	}
	return request.TriggerType
}

func (request CollectionRequest) Validate() error {
	if request.SourceConnectionID <= 0 || !validSHA256(request.QuerySignature) {
		return fmt.Errorf("collection request source and query signature are required")
	}
	if strings.TrimSpace(request.Query) == "" {
		return fmt.Errorf("collection request query is required")
	}
	if request.WindowStart.IsZero() || request.WindowEnd.IsZero() || !request.WindowEnd.After(request.WindowStart) {
		return fmt.Errorf("collection request window is invalid")
	}
	if !request.EffectiveTriggerType().Valid() {
		return fmt.Errorf("collection request trigger type is invalid")
	}
	if len(request.Targets) == 0 {
		return fmt.Errorf("collection request requires at least one target")
	}
	for _, target := range request.Targets {
		if err := target.Validate(); err != nil {
			return err
		}
		if target.SourceConnectionID != request.SourceConnectionID || target.QuerySignature != request.QuerySignature {
			return fmt.Errorf("collection request target does not share request identity")
		}
	}
	return nil
}

type CollectionRunStatus string

const (
	CollectionRunQueued    CollectionRunStatus = "queued"
	CollectionRunRunning   CollectionRunStatus = "running"
	CollectionRunSucceeded CollectionRunStatus = "succeeded"
	CollectionRunFailed    CollectionRunStatus = "failed"
	CollectionRunCancelled CollectionRunStatus = "cancelled"
)

type CollectionRun struct {
	ID                 int64
	SourceConnectionID int64
	QuerySignature     string
	RequestCursor      string
	NextCursor         string
	ETag               string
	LastModified       string
	RetryAfter         *time.Time
	PageCount          int
	WindowStart        time.Time
	WindowEnd          time.Time
	ScheduledAt        time.Time
	TriggerType        CollectionTriggerType
	Status             CollectionRunStatus
}

type ManualCollectionTargetReader interface {
	ListForManualCollection(context.Context, int64) ([]PublishedCollectionTarget, error)
}

type ManualCollectionCommand struct {
	SourceConnectionID int64
	ConfigVersionID    int64
	QuerySignature     string
	WindowStart        time.Time
	WindowEnd          time.Time
	ScheduledAt        time.Time
}

func (command ManualCollectionCommand) Validate() error {
	if command.SourceConnectionID <= 0 || command.ConfigVersionID <= 0 || !validSHA256(command.QuerySignature) {
		return fmt.Errorf("manual collection identity is invalid")
	}
	if command.WindowStart.IsZero() || command.WindowEnd.IsZero() || !command.WindowEnd.After(command.WindowStart) || command.ScheduledAt.IsZero() {
		return fmt.Errorf("manual collection time is invalid")
	}
	return nil
}

type ManualCollectionSummary struct {
	Requested     int
	Created       int
	Reused        int
	CooldownUntil time.Time
}

// CollectionRunRetry carries the immutable target identity captured with the
// original run. Retry infrastructure must prove that the complete target set
// is still eligible before it can reactivate the durable queue job.
type CollectionRunRetry struct {
	Run     CollectionRun
	Targets []CollectionRunTargetIdentity
}

type CollectionRunTargetIdentity struct {
	MonitorSourceID        int64
	MonitorConfigVersionID int64
}

// CollectionRunSummary is the deliberately safe operations projection. It
// excludes the source identity, query signature and all upstream request
// state, which remain internal collection execution facts.
type CollectionRunSummary struct {
	ID             int64
	Status         CollectionRunStatus
	CandidateCount int64
	AcceptedCount  int64
	RejectedCount  int64
	ErrorCode      string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	Targets        []CollectionRunTargetSummary
}

type CollectionRunTargetSummary struct {
	ID             int64
	Status         CollectionRunStatus
	CandidateCount int64
	AcceptedCount  int64
	RejectedCount  int64
	ErrorCode      string
}

type CollectionRunListQuery struct {
	Cursor string
	Limit  int
}

type CollectionRunPage struct {
	Items      []CollectionRunSummary
	NextCursor string
}

// CapturedItemQuery is the fixed-shape Source-owned reader input used by
// ingestion. It permits retrying classified ingestion failures only when the
// caller explicitly requests them; normal readers consume pending captures.
type CapturedItemQuery struct {
	RunID         int64
	Cursor        string
	Limit         int
	IncludeFailed bool
}

func (query CapturedItemQuery) Validate() error {
	if query.RunID <= 0 {
		return fmt.Errorf("captured item run id is required")
	}
	if query.Limit < 0 || query.Limit > 200 {
		return fmt.Errorf("captured item limit must be 0 (default) or 1-200")
	}
	return nil
}

// CapturedCollectionItem is the durable capture projection handed to
// ingestion. It never carries a connector, raw upstream response, target
// outcome, or any Monitor-owned state.
type CapturedCollectionItem struct {
	ID, RunID, SourceConnectionID int64
	Item                          CapturedItem
}

type CapturedItemPage struct {
	Items      []CapturedCollectionItem
	NextCursor string
}

type CapturedContentBinding struct {
	CollectionItemID, RunID, SourceConnectionID, ContentID int64
}

func (binding CapturedContentBinding) Validate() error {
	if binding.CollectionItemID <= 0 || binding.RunID <= 0 || binding.SourceConnectionID <= 0 || binding.ContentID <= 0 {
		return fmt.Errorf("captured content binding is incomplete")
	}
	return nil
}

type CapturedIngestionFailure struct {
	CollectionItemID, RunID, SourceConnectionID int64
	Code                                        string
}

func (failure CapturedIngestionFailure) Validate() error {
	if failure.CollectionItemID <= 0 || failure.RunID <= 0 || failure.SourceConnectionID <= 0 {
		return fmt.Errorf("captured ingestion failure is incomplete")
	}
	failure.Code = strings.TrimSpace(failure.Code)
	if failure.Code == "" || len(failure.Code) > 64 {
		return fmt.Errorf("captured ingestion failure code is invalid")
	}
	return nil
}

// SourceHealth is the safe result of an administrator-triggered Connector
// probe. Diagnostic codes are controlled Connector vocabulary, never upstream
// response text, endpoint values or credential facts.
type SourceHealth struct {
	Healthy   bool
	CheckedAt time.Time
	ErrorCode string
}

type CollectionTarget struct {
	ID                     int64
	CollectionRunID        int64
	MonitorSourceID        int64
	MonitorConfigVersionID int64
	Status                 CollectionRunStatus
}

func (target CollectionTarget) Validate() error {
	if target.CollectionRunID <= 0 || target.MonitorSourceID <= 0 || target.MonitorConfigVersionID <= 0 {
		return fmt.Errorf("collection target requires run and immutable published configuration")
	}
	return nil
}

type CollectionTargetItem struct {
	ID                    int64
	CollectionRunID       int64
	CollectionRunTargetID int64
	CollectionRunItemID   int64
	Outcome               string
	ReasonCode            string
}

// CollectionRunSuccess contains only already-captured safe items. The
// application invokes CapturePolicy before creating this command, so Source
// persistence never receives raw upstream response bytes.
type CollectionRunSuccess struct {
	RunID       int64
	Targets     []PublishedCollectionTarget
	Items       []CapturedItem
	Result      FetchResult
	CompletedAt time.Time
}

// CollectionRunFailure records a classified upstream failure without copying
// transport error strings into durable collection facts.
type CollectionRunFailure struct {
	RunID       int64
	Targets     []PublishedCollectionTarget
	Result      FetchResult
	ErrorKind   CollectionErrorKind
	CompletedAt time.Time
}

type CollectionCheckpoint struct {
	ID                  int64
	Version             int64
	MonitorSourceID     int64
	QueryHash           string
	CursorValue         string
	ETag                string
	LastModified        string
	HighWatermark       *time.Time
	LastSuccessfulRunID *int64
	LastFetchedAt       *time.Time
	NextPollAt          time.Time
	ConsecutiveFailures int
}

func (checkpoint CollectionCheckpoint) Validate() error {
	if checkpoint.MonitorSourceID <= 0 || !validSHA256(checkpoint.QueryHash) || checkpoint.NextPollAt.IsZero() {
		return fmt.Errorf("collection checkpoint is incomplete")
	}
	if checkpoint.ConsecutiveFailures < 0 {
		return fmt.Errorf("collection checkpoint failures cannot be negative")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
