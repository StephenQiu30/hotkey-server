package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

type BodyOrigin string

const (
	BodyOriginAPIContent                  BodyOrigin = "api_content"
	BodyOriginFeedContent                 BodyOrigin = "feed_content"
	BodyOriginFeedSummary                 BodyOrigin = "feed_summary"
	BodyOriginStructuredArticleBody       BodyOrigin = "structured_article_body"
	BodyOriginAuthorizedPayloadExtraction BodyOrigin = "authorized_payload_extraction"
	BodyOriginPlatformPost                BodyOrigin = "platform_post"
	BodyOriginSearchSnippet               BodyOrigin = "search_snippet"
)

func (origin BodyOrigin) Valid() bool {
	switch origin {
	case BodyOriginAPIContent, BodyOriginFeedContent, BodyOriginFeedSummary,
		BodyOriginStructuredArticleBody, BodyOriginAuthorizedPayloadExtraction,
		BodyOriginPlatformPost, BodyOriginSearchSnippet:
		return true
	default:
		return false
	}
}

type BodyCompleteness string

const (
	BodyCompletenessFull         BodyCompleteness = "full"
	BodyCompletenessPartial      BodyCompleteness = "partial"
	BodyCompletenessSummary      BodyCompleteness = "summary"
	BodyCompletenessSnippet      BodyCompleteness = "snippet"
	BodyCompletenessMetadataOnly BodyCompleteness = "metadata_only"
	BodyCompletenessUnknown      BodyCompleteness = "unknown"
)

func (completeness BodyCompleteness) Valid() bool {
	switch completeness {
	case BodyCompletenessFull, BodyCompletenessPartial, BodyCompletenessSummary,
		BodyCompletenessSnippet, BodyCompletenessMetadataOnly, BodyCompletenessUnknown:
		return true
	default:
		return false
	}
}

type DocumentLifecycleState string

const (
	DocumentPolicyPending    DocumentLifecycleState = "policy_pending"
	DocumentPolicyBlocked    DocumentLifecycleState = "policy_blocked"
	DocumentRawPending       DocumentLifecycleState = "raw_pending"
	DocumentRawAvailable     DocumentLifecycleState = "raw_available"
	DocumentRawFailed        DocumentLifecycleState = "raw_failed"
	DocumentDerivedPending   DocumentLifecycleState = "derive_pending"
	DocumentDerivedAvailable DocumentLifecycleState = "derived_available"
	DocumentDerivedFailed    DocumentLifecycleState = "derive_failed"
	DocumentReadable         DocumentLifecycleState = "readable"
	DocumentRetentionBlocked DocumentLifecycleState = "retention_blocked"
	DocumentQuarantined      DocumentLifecycleState = "quarantined"
	DocumentTombstoned       DocumentLifecycleState = "tombstoned"
)

func (state DocumentLifecycleState) Valid() bool {
	switch state {
	case DocumentPolicyPending, DocumentPolicyBlocked, DocumentRawPending,
		DocumentRawAvailable, DocumentRawFailed, DocumentDerivedPending,
		DocumentDerivedAvailable, DocumentDerivedFailed, DocumentReadable,
		DocumentRetentionBlocked, DocumentQuarantined, DocumentTombstoned:
		return true
	default:
		return false
	}
}

type DocumentVersionCandidate struct {
	DocumentID              int64
	SourceObservationID     int64
	BodyOrigin              BodyOrigin
	Completeness            BodyCompleteness
	Body                    string
	Language                string
	ExtractorVersion        string
	ExtractorProfileVersion string
	ExtractorProfileSHA256  string
	CapturedAt              time.Time
	Truncated               bool
	QualityScore            *float64
	QualityWarnings         []string
}

// NormalizedDocumentVersion is an immutable write candidate. VersionKey is
// scoped to the exact source observation, normalized body and extractor
// profile version; a later observation remains a distinct provenance fact.
type NormalizedDocumentVersion struct {
	DocumentID              int64
	SourceObservationID     int64
	BodyOrigin              BodyOrigin
	Completeness            BodyCompleteness
	Body                    string
	Language                string
	ExtractorVersion        string
	ExtractorProfileVersion string
	ExtractorProfileSHA256  string
	CapturedAt              time.Time
	Truncated               bool
	QualityScore            *float64
	QualityWarnings         []string
	WordCount               int
	ContentSHA256           string
	VersionKey              string
	LifecycleState          DocumentLifecycleState
}

func (candidate DocumentVersionCandidate) Normalize() (NormalizedDocumentVersion, error) {
	if candidate.DocumentID <= 0 || candidate.SourceObservationID <= 0 {
		return NormalizedDocumentVersion{}, fmt.Errorf("document and source observation are required")
	}
	if !candidate.BodyOrigin.Valid() || !candidate.Completeness.Valid() {
		return NormalizedDocumentVersion{}, fmt.Errorf("body origin or completeness is invalid")
	}
	if err := validateBodySemantics(candidate.BodyOrigin, candidate.Completeness, candidate.Body); err != nil {
		return NormalizedDocumentVersion{}, err
	}
	if candidate.CapturedAt.IsZero() {
		return NormalizedDocumentVersion{}, fmt.Errorf("document capture time is required")
	}
	extractorVersion := strings.TrimSpace(candidate.ExtractorVersion)
	if extractorVersion == "" || len(extractorVersion) > 64 {
		return NormalizedDocumentVersion{}, fmt.Errorf("extractor version is invalid")
	}
	extractorProfileVersion := strings.TrimSpace(candidate.ExtractorProfileVersion)
	if extractorProfileVersion == "" || len(extractorProfileVersion) > 64 {
		return NormalizedDocumentVersion{}, fmt.Errorf("extractor profile version is invalid")
	}
	extractorProfileSHA256 := strings.ToLower(strings.TrimSpace(candidate.ExtractorProfileSHA256))
	if !validDocumentSHA256(extractorProfileSHA256) {
		return NormalizedDocumentVersion{}, fmt.Errorf("extractor profile SHA-256 is invalid")
	}
	languageValue := strings.TrimSpace(candidate.Language)
	if languageValue == "" {
		languageValue = "und"
	}
	tag, err := language.Parse(languageValue)
	if err != nil {
		return NormalizedDocumentVersion{}, fmt.Errorf("document language is invalid")
	}
	languageValue = tag.String()
	body := normalizeDocumentBody(candidate.Body)
	warnings, err := normalizeQualityWarnings(candidate.QualityWarnings)
	if err != nil {
		return NormalizedDocumentVersion{}, err
	}
	qualityScore, err := normalizeQualityScore(candidate.QualityScore)
	if err != nil {
		return NormalizedDocumentVersion{}, err
	}
	contentDigest := sha256.Sum256([]byte(body))
	contentSHA := hex.EncodeToString(contentDigest[:])
	identity := strings.Join([]string{
		fmt.Sprintf("%d", candidate.DocumentID), fmt.Sprintf("%d", candidate.SourceObservationID),
		contentSHA, extractorProfileVersion,
	}, "\n")
	versionDigest := sha256.Sum256([]byte(identity))
	return NormalizedDocumentVersion{
		DocumentID: candidate.DocumentID, SourceObservationID: candidate.SourceObservationID,
		BodyOrigin: candidate.BodyOrigin, Completeness: candidate.Completeness,
		Body: body, Language: languageValue, ExtractorVersion: extractorVersion,
		ExtractorProfileVersion: extractorProfileVersion, ExtractorProfileSHA256: extractorProfileSHA256,
		CapturedAt: candidate.CapturedAt.UTC(),
		Truncated:  candidate.Truncated, QualityScore: qualityScore,
		QualityWarnings: warnings, WordCount: len(strings.Fields(body)),
		ContentSHA256: contentSHA, VersionKey: hex.EncodeToString(versionDigest[:]), LifecycleState: DocumentPolicyPending,
	}, nil
}

func validDocumentSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validateBodySemantics(origin BodyOrigin, completeness BodyCompleteness, rawBody string) error {
	body := normalizeDocumentBody(rawBody)
	if completeness == BodyCompletenessMetadataOnly {
		if body != "" {
			return fmt.Errorf("metadata-only document cannot include a body")
		}
		return nil
	}
	if completeness != BodyCompletenessUnknown && body == "" {
		return fmt.Errorf("document completeness requires a body")
	}
	if origin == BodyOriginFeedSummary && completeness != BodyCompletenessSummary && completeness != BodyCompletenessMetadataOnly && completeness != BodyCompletenessUnknown {
		return fmt.Errorf("feed summary cannot be promoted to %s", completeness)
	}
	if origin == BodyOriginSearchSnippet && completeness != BodyCompletenessSnippet && completeness != BodyCompletenessMetadataOnly && completeness != BodyCompletenessUnknown {
		return fmt.Errorf("search snippet cannot be promoted to %s", completeness)
	}
	if completeness == BodyCompletenessFull {
		switch origin {
		case BodyOriginAPIContent, BodyOriginFeedContent, BodyOriginPlatformPost:
		default:
			return fmt.Errorf("body origin %s cannot assert full completeness", origin)
		}
	}
	return nil
}

func normalizeDocumentBody(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return norm.NFC.String(strings.TrimSpace(value))
}

func normalizeQualityScore(value *float64) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 100 {
		return nil, fmt.Errorf("document quality score is invalid")
	}
	normalized := math.Round(*value*100) / 100
	return &normalized, nil
}

func normalizeQualityWarnings(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if len(value) > 64 || len(normalized) >= 32 {
			return nil, fmt.Errorf("document quality warning is invalid")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func ValidateDocumentTransition(from, to DocumentLifecycleState) error {
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("document lifecycle transition uses an invalid state")
	}
	if from == to {
		return nil
	}
	allowed := map[DocumentLifecycleState]map[DocumentLifecycleState]bool{
		DocumentPolicyPending: {
			DocumentDerivedPending: true, DocumentPolicyBlocked: true, DocumentQuarantined: true,
		},
		DocumentDerivedPending: {
			DocumentDerivedAvailable: true, DocumentDerivedFailed: true, DocumentPolicyBlocked: true,
			DocumentQuarantined: true,
		},
		DocumentDerivedAvailable: {
			DocumentReadable: true, DocumentPolicyBlocked: true, DocumentQuarantined: true,
			DocumentTombstoned: true,
		},
		DocumentReadable: {
			DocumentPolicyBlocked: true, DocumentRetentionBlocked: true, DocumentQuarantined: true, DocumentTombstoned: true,
		},
		DocumentDerivedFailed: {DocumentDerivedPending: true, DocumentQuarantined: true, DocumentTombstoned: true},
		DocumentQuarantined:   {DocumentTombstoned: true},
		DocumentPolicyBlocked: {
			DocumentDerivedPending: true, DocumentReadable: true, DocumentQuarantined: true, DocumentTombstoned: true,
		},
		DocumentRetentionBlocked: {DocumentTombstoned: true},
	}
	if !allowed[from][to] {
		return fmt.Errorf("document lifecycle transition %s -> %s is not allowed", from, to)
	}
	return nil
}
