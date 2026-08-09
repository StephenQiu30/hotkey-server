package postgres

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

const evidenceSnapshotColumns = `
id,lifecycle_state,source_connection_id,collection_run_id,
store_raw_rights_decision_id,retain_rights_decision_id,
snapshot_key,object_key,payload_sha256,collector_profile_version,
mime_type,size_bytes,response_status,requested_url,final_url,
redirect_chain,response_headers,captured_at,retention_until,
failure_code,available_at`

type evidenceSnapshotRecord struct {
	ID                      int64
	LifecycleState          string
	SourceConnectionID      int64
	CollectionRunID         sql.NullInt64
	StoreRawDecisionID      int64
	RetainDecisionID        int64
	EvidenceKey             string
	ObjectKey               string
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
	SizeBytes               int64
	ResponseStatus          int
	RequestedURL            string
	FinalURL                string
	RedirectChainJSON       []byte
	ResponseHeadersJSON     []byte
	CapturedAt              time.Time
	RetentionUntil          time.Time
	FailureCode             sql.NullString
	AvailableAt             sql.NullTime
}

type evidenceSnapshotScanner interface {
	Scan(...any) error
}

func scanEvidenceSnapshotRecord(scanner evidenceSnapshotScanner) (evidenceSnapshotRecord, error) {
	var record evidenceSnapshotRecord
	err := scanner.Scan(
		&record.ID, &record.LifecycleState, &record.SourceConnectionID, &record.CollectionRunID,
		&record.StoreRawDecisionID, &record.RetainDecisionID,
		&record.EvidenceKey, &record.ObjectKey, &record.PayloadSHA256, &record.CollectorProfileVersion,
		&record.MIMEType, &record.SizeBytes, &record.ResponseStatus, &record.RequestedURL, &record.FinalURL,
		&record.RedirectChainJSON, &record.ResponseHeadersJSON, &record.CapturedAt, &record.RetentionUntil,
		&record.FailureCode, &record.AvailableAt,
	)
	return record, err
}

func (record evidenceSnapshotRecord) persistenceDTO() (sourceapplication.PersistedEvidenceSnapshotDTO, error) {
	redirectChain, err := decodeRedirectChain(record.RedirectChainJSON)
	if err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, err
	}
	responseHeaders, err := decodeResponseHeaders(record.ResponseHeadersJSON)
	if err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, err
	}
	return sourceapplication.PersistedEvidenceSnapshotDTO{
		ID: record.ID, LifecycleState: record.LifecycleState,
		SourceConnectionID: record.SourceConnectionID, CollectionRunID: record.CollectionRunID.Int64,
		StoreRawRightsDecisionID: record.StoreRawDecisionID, RetainRightsDecisionID: record.RetainDecisionID,
		EvidenceKey: strings.TrimSpace(record.EvidenceKey), ObjectKey: record.ObjectKey,
		PayloadSHA256: strings.TrimSpace(record.PayloadSHA256), CollectorProfileVersion: record.CollectorProfileVersion,
		MIMEType: record.MIMEType, SizeBytes: record.SizeBytes, ResponseStatus: record.ResponseStatus,
		RequestedURL: record.RequestedURL, FinalURL: record.FinalURL, RedirectChain: redirectChain,
		ResponseHeaders: responseHeaders, CapturedAt: record.CapturedAt.UTC(), RetentionUntil: record.RetentionUntil.UTC(),
	}, nil
}

func sameEvidenceIdentity(record evidenceSnapshotRecord, command sourceapplication.ReserveEvidenceSnapshotCommand) bool {
	return record.SourceConnectionID == command.SourceConnectionID && strings.TrimSpace(record.EvidenceKey) == command.EvidenceKey &&
		record.ObjectKey == command.ObjectKey && strings.TrimSpace(record.PayloadSHA256) == command.PayloadSHA256 &&
		record.CollectorProfileVersion == command.CollectorProfileVersion && record.SizeBytes == command.SizeBytes
}

func evidenceStoreResultMatches(record evidenceSnapshotRecord, result sourceapplication.StoreRawEvidenceResult) bool {
	return record.SourceConnectionID == result.SourceConnectionID && strings.TrimSpace(record.EvidenceKey) == result.EvidenceKey &&
		record.ObjectKey == result.ObjectKey && strings.TrimSpace(record.PayloadSHA256) == result.PayloadSHA256 &&
		record.CollectorProfileVersion == result.CollectorProfileVersion && record.MIMEType == result.MIMEType &&
		record.SizeBytes == result.SizeBytes
}

type sourceObservationRecord struct {
	ID                  int64
	SourceConnectionID  int64
	CollectionRunItemID sql.NullInt64
	ExternalID          string
	UpstreamIdentity    string
	SourceCode          string
	ContentType         string
	Title               string
	Language            string
	Author              sql.NullString
	SourceRecordURL     sql.NullString
	CanonicalURL        sql.NullString
	DiscussionURL       sql.NullString
	BodyOrigin          string
	Completeness        string
	PublishedAt         sql.NullTime
	DiscoveredAt        time.Time
	CapturedAt          time.Time
}

const sourceObservationColumns = `
id,source_connection_id,collection_run_item_id,external_id,upstream_identity,
source_code,content_type,title,language,author_snapshot,source_record_url,
canonical_url,discussion_url,body_origin,completeness,published_at,
discovered_at,captured_at`

func scanSourceObservationRecord(scanner evidenceSnapshotScanner) (sourceObservationRecord, error) {
	var record sourceObservationRecord
	err := scanner.Scan(
		&record.ID, &record.SourceConnectionID, &record.CollectionRunItemID, &record.ExternalID, &record.UpstreamIdentity,
		&record.SourceCode, &record.ContentType, &record.Title, &record.Language, &record.Author,
		&record.SourceRecordURL, &record.CanonicalURL, &record.DiscussionURL, &record.BodyOrigin, &record.Completeness,
		&record.PublishedAt, &record.DiscoveredAt, &record.CapturedAt,
	)
	return record, err
}

// sameObservationContentFacts compares normalized content facts while leaving
// first-seen collection/capture receipt facts immutable across recaptures.
// Evidence-specific selected hashes are compared on the M:N locator record;
// one observation may legitimately select different bytes from two snapshots.
func sameObservationContentFacts(record sourceObservationRecord, observation sourceapplication.SourceObservationDTO) bool {
	return record.SourceConnectionID == observation.SourceConnectionID && record.ExternalID == observation.ExternalID &&
		strings.TrimSpace(record.UpstreamIdentity) == observation.UpstreamIdentity && record.SourceCode == observation.SourceCode &&
		record.ContentType == observation.ContentType && record.Title == observation.Title && record.Language == observation.Language &&
		nullStringEquals(record.Author, observation.Author) && nullStringEquals(record.CanonicalURL, observation.CanonicalURL) &&
		nullStringEquals(record.DiscussionURL, observation.DiscussionURL) && record.BodyOrigin == observation.BodyOrigin &&
		record.Completeness == observation.Completeness && nullTimeEquals(record.PublishedAt, observation.PublishedAt)
}

type evidenceLocatorRecord struct {
	ID                    int64
	SourceConnectionID    int64
	SourceObservationID   int64
	EvidenceSnapshotID    int64
	LocatorType           string
	LocatorValue          string
	ByteStart             sql.NullInt64
	ByteEnd               sql.NullInt64
	SelectedPayloadSHA256 string
	SelectorVersion       string
}

const evidenceLocatorColumns = `
id,source_connection_id,source_observation_id,evidence_snapshot_id,
locator_type,locator_value,byte_start,byte_end,selected_payload_sha256,selector_version`

func scanEvidenceLocatorRecord(scanner evidenceSnapshotScanner) (evidenceLocatorRecord, error) {
	var record evidenceLocatorRecord
	err := scanner.Scan(
		&record.ID, &record.SourceConnectionID, &record.SourceObservationID, &record.EvidenceSnapshotID,
		&record.LocatorType, &record.LocatorValue, &record.ByteStart, &record.ByteEnd,
		&record.SelectedPayloadSHA256, &record.SelectorVersion,
	)
	return record, err
}

func sameEvidenceLocatorFacts(record evidenceLocatorRecord, sourceConnectionID, observationID, snapshotID int64, reference sourceapplication.RawEvidenceReferenceDTO) bool {
	return record.SourceConnectionID == sourceConnectionID && record.SourceObservationID == observationID &&
		record.EvidenceSnapshotID == snapshotID && record.LocatorType == reference.LocatorType &&
		record.LocatorValue == reference.LocatorValue && nullInt64Equals(record.ByteStart, reference.ByteStart) &&
		nullInt64Equals(record.ByteEnd, reference.ByteEnd) && strings.TrimSpace(record.SelectedPayloadSHA256) == reference.SelectedPayloadSHA256 &&
		record.SelectorVersion == reference.SelectorVersion
}

func (record evidenceLocatorRecord) committedReferenceDTO() sourceapplication.CommittedEvidenceReferenceDTO {
	return sourceapplication.CommittedEvidenceReferenceDTO{
		EvidenceReferenceID: record.ID,
		SourceObservationID: record.SourceObservationID,
		EvidenceSnapshotID:  record.EvidenceSnapshotID,
	}
}

func validateEvidenceReservation(command sourceapplication.ReserveEvidenceSnapshotCommand) error {
	if command.SourceConnectionID <= 0 || command.CollectionRunID <= 0 || command.StoreRawRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		!validSHA256Record(command.EvidenceKey) || !validSHA256Record(command.PayloadSHA256) || command.SizeBytes < 0 ||
		command.ResponseStatus < 100 || command.ResponseStatus > 599 || command.CapturedAt.IsZero() ||
		!command.RetentionUntil.After(command.CapturedAt) {
		return fmt.Errorf("evidence reservation identity or lifetime is invalid")
	}
	profile, err := domain.NewCollectorProfileVersion(command.CollectorProfileVersion)
	if err != nil {
		return fmt.Errorf("evidence reservation collector profile is invalid")
	}
	identity, err := domain.EvidenceSnapshotIdentity(command.PayloadSHA256, profile)
	if err != nil || identity != command.EvidenceKey || command.ObjectKey != sourceapplication.RawEvidenceObjectKey(command.SourceConnectionID, command.EvidenceKey) {
		return fmt.Errorf("evidence reservation endpoint-scoped identity is invalid")
	}
	if err := validateEvidenceMIME(command.MIMEType); err != nil {
		return err
	}
	if err := validateEvidenceURL(command.RequestedURL, true); err != nil {
		return fmt.Errorf("evidence reservation requested URL is invalid")
	}
	if err := validateEvidenceURL(command.FinalURL, true); err != nil {
		return fmt.Errorf("evidence reservation final URL is invalid")
	}
	if len(command.RedirectChain) > 10 {
		return fmt.Errorf("evidence reservation redirect chain is invalid")
	}
	for _, redirect := range command.RedirectChain {
		if err := validateEvidenceURL(redirect, true); err != nil {
			return fmt.Errorf("evidence reservation redirect chain is invalid")
		}
	}
	if command.RequestedURL == command.FinalURL {
		if len(command.RedirectChain) != 0 {
			return fmt.Errorf("evidence reservation redirect chain is invalid")
		}
	} else if len(command.RedirectChain) == 0 || command.RedirectChain[len(command.RedirectChain)-1] != command.FinalURL {
		return fmt.Errorf("evidence reservation redirect chain is invalid")
	}
	if values := command.ResponseHeaders.Values()["Content-Type"]; len(values) == 1 && values[0] != command.MIMEType {
		return fmt.Errorf("evidence reservation Content-Type does not match MIME type")
	}
	return nil
}

func validateEvidenceStoreResult(result sourceapplication.StoreRawEvidenceResult) error {
	if result.SourceConnectionID <= 0 || !validSHA256Record(result.EvidenceKey) || !validSHA256Record(result.PayloadSHA256) ||
		result.ObjectKey != sourceapplication.RawEvidenceObjectKey(result.SourceConnectionID, result.EvidenceKey) || result.SizeBytes < 0 {
		return fmt.Errorf("raw evidence store result identity is invalid")
	}
	profile, err := domain.NewCollectorProfileVersion(result.CollectorProfileVersion)
	if err != nil {
		return fmt.Errorf("raw evidence store result collector profile is invalid")
	}
	identity, err := domain.EvidenceSnapshotIdentity(result.PayloadSHA256, profile)
	if err != nil || identity != result.EvidenceKey {
		return fmt.Errorf("raw evidence store result identity is invalid")
	}
	return validateEvidenceMIME(result.MIMEType)
}

func validateSourceObservation(observation sourceapplication.SourceObservationDTO, snapshot evidenceSnapshotRecord) error {
	if observation.SourceConnectionID != snapshot.SourceConnectionID || observation.CollectionRunID <= 0 ||
		observation.ExternalID == "" || observation.ExternalID != strings.TrimSpace(observation.ExternalID) || len(observation.ExternalID) > 512 ||
		!validSHA256Record(observation.UpstreamIdentity) || observation.SourceCode == "" || len(observation.SourceCode) > 64 ||
		observation.ContentType == "" || len(observation.ContentType) > 32 || len(observation.Title) > 1<<20 ||
		observation.Language == "" || len(observation.Language) > 32 || len(observation.Author) > 512 ||
		observation.DiscoveredAt.IsZero() || observation.CapturedAt.IsZero() || observation.CapturedAt.Before(observation.DiscoveredAt) {
		return fmt.Errorf("source observation identity or time is invalid")
	}
	if strings.ContainsAny(observation.ExternalID+observation.SourceCode+observation.ContentType+observation.Language+observation.Author, "\x00\r\n") {
		return fmt.Errorf("source observation text metadata is invalid")
	}
	if observation.SourceRecordURL == "" && observation.CanonicalURL == "" && observation.DiscussionURL == "" {
		return fmt.Errorf("source observation requires a source URL")
	}
	for _, value := range []string{observation.SourceRecordURL, observation.CanonicalURL, observation.DiscussionURL} {
		if value != "" {
			if err := validateEvidenceURL(value, false); err != nil {
				return fmt.Errorf("source observation URL is invalid")
			}
		}
	}
	switch observation.BodyOrigin {
	case "api_content", "feed_content", "feed_summary", "structured_article_body", "authorized_payload_extraction", "platform_post", "search_snippet":
	default:
		return fmt.Errorf("source observation body origin is invalid")
	}
	switch observation.Completeness {
	case "full", "partial", "summary", "snippet", "metadata_only", "unknown":
	default:
		return fmt.Errorf("source observation completeness is invalid")
	}
	if observation.Evidence.EvidenceKey != strings.TrimSpace(snapshot.EvidenceKey) || len(observation.Evidence.SelectorVersion) > 64 {
		return fmt.Errorf("source observation evidence identity is invalid")
	}
	if err := observation.Evidence.Validate(); err != nil {
		return fmt.Errorf("source observation evidence is invalid: %w", err)
	}
	return nil
}

func encodeRedirectChain(values []string) ([]byte, error) {
	redirectChain := make([]string, len(values))
	copy(redirectChain, values)
	encoded, err := json.Marshal(redirectChain)
	if err != nil {
		return nil, fmt.Errorf("encode evidence redirect chain: %w", err)
	}
	return encoded, nil
}

func decodeRedirectChain(encoded []byte) ([]string, error) {
	var values []string
	if err := json.Unmarshal(encoded, &values); err != nil || values == nil {
		return nil, fmt.Errorf("decode evidence redirect chain")
	}
	return values, nil
}

var lowercaseResponseHeaderNames = map[string]string{
	"Content-Type":  "content-type",
	"ETag":          "etag",
	"Last-Modified": "last-modified",
	"Date":          "date",
	"Link":          "link",
	"Retry-After":   "retry-after",
}

var titleCaseResponseHeaderNames = map[string]string{
	"content-type":  "Content-Type",
	"etag":          "ETag",
	"last-modified": "Last-Modified",
	"date":          "Date",
	"link":          "Link",
	"retry-after":   "Retry-After",
}

func encodeResponseHeaders(headers sourceapplication.RawResponseHeadersDTO) ([]byte, error) {
	if err := headers.Validate(); err != nil {
		return nil, fmt.Errorf("encode evidence response headers: %w", err)
	}
	lowercase := make(map[string][]string, len(headers.Values()))
	for titleCaseName, values := range headers.Values() {
		lowercaseName, allowed := lowercaseResponseHeaderNames[titleCaseName]
		if !allowed {
			return nil, fmt.Errorf("encode evidence response headers: unsupported header")
		}
		lowercase[lowercaseName] = append([]string(nil), values...)
	}
	encoded, err := json.Marshal(lowercase)
	if err != nil {
		return nil, fmt.Errorf("encode evidence response headers: %w", err)
	}
	return encoded, nil
}

func decodeResponseHeaders(encoded []byte) (sourceapplication.RawResponseHeadersDTO, error) {
	var lowercase map[string][]string
	if err := json.Unmarshal(encoded, &lowercase); err != nil || lowercase == nil {
		return sourceapplication.RawResponseHeadersDTO{}, fmt.Errorf("decode evidence response headers")
	}
	titleCase := make(map[string][]string, len(lowercase))
	for lowercaseName, values := range lowercase {
		titleCaseName, allowed := titleCaseResponseHeaderNames[lowercaseName]
		if !allowed {
			return sourceapplication.RawResponseHeadersDTO{}, fmt.Errorf("decode evidence response headers: unsupported header")
		}
		titleCase[titleCaseName] = append([]string(nil), values...)
	}
	headers, err := sourceapplication.NewRawResponseHeadersDTO(titleCase)
	if err != nil {
		return sourceapplication.RawResponseHeadersDTO{}, fmt.Errorf("decode evidence response headers: %w", err)
	}
	return headers, nil
}

func validSHA256Record(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validateEvidenceMIME(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 255 || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("evidence MIME type is invalid")
	}
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil || !strings.Contains(mediaType, "/") {
		return fmt.Errorf("evidence MIME type is invalid")
	}
	mediaType = strings.ToLower(mediaType)
	if charset, found := parameters["charset"]; found {
		parameters["charset"] = strings.ToLower(charset)
	}
	if mime.FormatMediaType(mediaType, parameters) != value {
		return fmt.Errorf("evidence MIME type is not canonical")
	}
	return nil
}

func validateEvidenceURL(value string, httpsOnly bool) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("invalid URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid URL")
	}
	if httpsOnly && parsed.Scheme != "https" {
		return fmt.Errorf("invalid URL")
	}
	if !httpsOnly && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid URL")
	}
	return nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullStringEquals(stored sql.NullString, value string) bool {
	return stored.Valid == (value != "") && (!stored.Valid || stored.String == value)
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func nullTimeEquals(stored sql.NullTime, value *time.Time) bool {
	return stored.Valid == (value != nil) &&
		(!stored.Valid || stored.Time.Equal(value.UTC().Truncate(time.Microsecond)))
}

func nullInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullInt64Equals(stored sql.NullInt64, value *int64) bool {
	return stored.Valid == (value != nil) && (!stored.Valid || stored.Int64 == *value)
}
