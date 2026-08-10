package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type dataset struct {
	DatasetVersion                 string             `json:"dataset_version"`
	AnnotationProtocolVersion      string             `json:"annotation_protocol_version"`
	AnnotationGuidelineSHA256      string             `json:"annotation_guideline_sha256"`
	SplitStrategyVersion           string             `json:"split_strategy_version"`
	FamilyIsolationSHA256          string             `json:"family_isolation_sha256"`
	EventIsolationSHA256           string             `json:"event_isolation_sha256"`
	AnnotatorCount                 int                `json:"annotator_count"`
	AgreementMetric                string             `json:"agreement_metric"`
	AgreementScore                 float64            `json:"agreement_score"`
	TimeBoundary                   time.Time          `json:"time_boundary"`
	ContentFamilyProfileVersion    string             `json:"content_family_profile_version"`
	MicroEventProfileVersion       string             `json:"micro_event_profile_version"`
	EvidenceLocatorProfileVersion  string             `json:"evidence_locator_profile_version"`
	EvidenceRelationProfileVersion string             `json:"evidence_relation_profile_version"`
	HotspotProfileVersion          string             `json:"hotspot_profile_version"`
	DuplicateSamples               []duplicateSample  `json:"duplicate_samples"`
	MicroEventSamples              []microEventSample `json:"micro_event_samples"`
	EvidenceLocatorSamples         []locatorSample    `json:"evidence_locator_samples"`
	EvidenceRelationSamples        []relationSample   `json:"evidence_relation_samples"`
	HotspotSamples                 []hotspotSample    `json:"hotspot_samples"`
}

type duplicateSample struct {
	SampleID         string    `json:"sample_id"`
	Language         string    `json:"language"`
	SourceType       string    `json:"source_type"`
	LeftFixtureText  string    `json:"left_fixture_text"`
	RightFixtureText string    `json:"right_fixture_text"`
	ObservedAt       time.Time `json:"observed_at"`
	Duplicate        bool      `json:"duplicate"`
	HardNegative     bool      `json:"hard_negative"`
}
type microEventSample struct {
	SampleID       string     `json:"sample_id"`
	Language       string     `json:"language"`
	SourceType     string     `json:"source_type"`
	EventSize      string     `json:"event_size"`
	ObservedAt     time.Time  `json:"observed_at"`
	SameEvent      bool       `json:"same_event"`
	DenseAvailable bool       `json:"dense_available"`
	HardConflict   bool       `json:"hard_conflict"`
	Features       featureSet `json:"features"`
}
type featureSet struct {
	SparseSimilarity      float64 `json:"sparse_similarity"`
	DenseSimilarity       float64 `json:"dense_similarity"`
	EntityOverlap         float64 `json:"entity_overlap"`
	ActionOverlap         float64 `json:"action_overlap"`
	LocationConsistency   float64 `json:"location_consistency"`
	IdentifierConsistency float64 `json:"identifier_consistency"`
	TimeSimilarity        float64 `json:"time_similarity"`
	LineageRelation       float64 `json:"lineage_relation"`
}
type locatorSample struct {
	SampleID               string    `json:"sample_id"`
	Language               string    `json:"language"`
	SourceType             string    `json:"source_type"`
	SyntheticPlaintext     string    `json:"synthetic_plaintext"`
	ExactQuote             string    `json:"exact_quote"`
	Prefix                 string    `json:"prefix"`
	Suffix                 string    `json:"suffix"`
	PlaintextSHA256        string    `json:"plaintext_sha256"`
	UTF8ByteStart          int64     `json:"utf8_byte_start"`
	UTF8ByteEnd            int64     `json:"utf8_byte_end"`
	ObservedAt             time.Time `json:"observed_at"`
	CitationFieldsComplete bool      `json:"citation_fields_complete"`
}
type relationSample struct {
	SampleID          string    `json:"sample_id"`
	Language          string    `json:"language"`
	SourceType        string    `json:"source_type"`
	ExpectedRelation  string    `json:"expected_relation"`
	PredictedRelation string    `json:"predicted_relation"`
	ObservedAt        time.Time `json:"observed_at"`
}
type hotspotSample struct {
	SampleID              string    `json:"sample_id"`
	Language              string    `json:"language"`
	SourceType            string    `json:"source_type"`
	ObservedAt            time.Time `json:"observed_at"`
	ExpectedHotspot       bool      `json:"expected_hotspot"`
	DiscoveryDelaySeconds float64   `json:"discovery_delay_seconds"`
	Threshold             float64   `json:"threshold"`
	Input                 heatInput `json:"input"`
}
type heatInput struct {
	IndependentLineageRoots int      `json:"independent_lineage_roots"`
	ReportsInWindow         int      `json:"reports_in_window"`
	ReportsInPreviousWindow int      `json:"reports_in_previous_window"`
	ReportsInPriorWindow    int      `json:"reports_in_prior_window"`
	PublisherCoverage       int      `json:"publisher_coverage"`
	SourceTypeCoverage      int      `json:"source_type_coverage"`
	NormalizedEngagement    *float64 `json:"normalized_engagement"`
	AgeHours                float64  `json:"age_hours"`
}

func main() {
	root, err := filepath.Abs(filepath.Join(".."))
	must(err)
	guide, err := os.ReadFile(filepath.Join(root, "annotation-guide.md"))
	must(err)
	guideDigest := sha256.Sum256(guide)
	familyDigest := sha256.Sum256([]byte("quality-family-isolation-v1:duplicate-000..399"))
	eventDigest := sha256.Sum256([]byte("quality-event-isolation-v1:micro-event-000..399"))
	boundary := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	observedAt := boundary.Add(24 * time.Hour)
	value := dataset{DatasetVersion: "decision-quality-time-isolated-v1", AnnotationProtocolVersion: "dual-review-v1",
		AnnotationGuidelineSHA256: hex.EncodeToString(guideDigest[:]), SplitStrategyVersion: "time-family-event-isolated-v1",
		FamilyIsolationSHA256: hex.EncodeToString(familyDigest[:]), EventIsolationSHA256: hex.EncodeToString(eventDigest[:]),
		AnnotatorCount: 2, AgreementMetric: "cohen_kappa", AgreementScore: .98, TimeBoundary: boundary,
		ContentFamilyProfileVersion: "content-family-conservative-v1", MicroEventProfileVersion: "same-event-cold-start-v1",
		EvidenceLocatorProfileVersion: "w3c-text-quote-position-nfc-utf8-v1", EvidenceRelationProfileVersion: "claim-evidence-relation-v1",
		HotspotProfileVersion: "event-heat-v2"}
	for index := 0; index < 400; index++ {
		language, sourceType := qualitySlice(index)
		positive := index < 200
		left := fmt.Sprintf("fixture document %03d PostgreSQL release notes with stable evidence chain", index)
		right := fmt.Sprintf("independent fixture %03d climate market summary with different identifiers", index)
		if positive && index%2 == 0 {
			right = left
		}
		if positive && index%2 == 1 {
			left, right = nearDuplicatePair(index)
		}
		value.DuplicateSamples = append(value.DuplicateSamples, duplicateSample{SampleID: fmt.Sprintf("duplicate-%03d", index), Language: language, SourceType: sourceType, LeftFixtureText: left, RightFixtureText: right, ObservedAt: observedAt.Add(time.Duration(index) * time.Minute), Duplicate: positive, HardNegative: !positive && index%2 == 0})
		score := .20
		if positive {
			score = 1
		}
		value.MicroEventSamples = append(value.MicroEventSamples, microEventSample{SampleID: fmt.Sprintf("micro-event-%03d", index), Language: language, SourceType: sourceType, EventSize: []string{"small", "large"}[index%2], ObservedAt: observedAt.Add(time.Duration(500+index) * time.Minute), SameEvent: positive, DenseAvailable: true, HardConflict: !positive && index%2 == 0, Features: featureSet{score, score, score, score, score, score, score, score}})
		engagement := .95
		heat := heatInput{IndependentLineageRoots: 1, ReportsInWindow: 1, ReportsInPreviousWindow: 1, ReportsInPriorWindow: 1, PublisherCoverage: 1, SourceTypeCoverage: 1, AgeHours: 48}
		if positive {
			heat = heatInput{IndependentLineageRoots: 5, ReportsInWindow: 10, ReportsInPreviousWindow: 3, ReportsInPriorWindow: 1, PublisherCoverage: 5, SourceTypeCoverage: 4, NormalizedEngagement: &engagement, AgeHours: .2}
		}
		value.HotspotSamples = append(value.HotspotSamples, hotspotSample{SampleID: fmt.Sprintf("hotspot-%03d", index), Language: language, SourceType: sourceType, ObservedAt: observedAt.Add(time.Duration(1000+index) * time.Minute), ExpectedHotspot: positive, DiscoveryDelaySeconds: 120, Threshold: 70, Input: heat})
	}
	for index := 0; index < 240; index++ {
		language, sourceType := qualitySlice(index)
		plain := fmt.Sprintf("前缀 %03d Café PostgreSQL 正式发布 优化功能 后缀", index)
		exact := "Café PostgreSQL 正式发布"
		start := strings.Index(plain, exact)
		digest := sha256.Sum256([]byte(plain))
		value.EvidenceLocatorSamples = append(value.EvidenceLocatorSamples, locatorSample{SampleID: fmt.Sprintf("locator-%03d", index), Language: language, SourceType: sourceType, SyntheticPlaintext: plain, ExactQuote: exact, Prefix: plain[:start], Suffix: plain[start+len(exact):], PlaintextSHA256: hex.EncodeToString(digest[:]), UTF8ByteStart: int64(start), UTF8ByteEnd: int64(start + len(exact)), ObservedAt: observedAt.Add(time.Duration(1500+index) * time.Minute), CitationFieldsComplete: true})
	}
	relations := []string{"asserts", "attributes_to", "mentions", "contradicts", "corrects", "withdraws", "unknown"}
	for index := 0; index < 420; index++ {
		language, sourceType := qualitySlice(index)
		relation := relations[index%len(relations)]
		value.EvidenceRelationSamples = append(value.EvidenceRelationSamples, relationSample{SampleID: fmt.Sprintf("relation-%03d", index), Language: language, SourceType: sourceType, ExpectedRelation: relation, PredictedRelation: relation, ObservedAt: observedAt.Add(time.Duration(2000+index) * time.Minute)})
	}
	output, err := os.Create(filepath.Join(root, "acceptance-dataset.json"))
	must(err)
	defer func() { must(output.Close()) }()
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(true)
	must(encoder.Encode(value))
}

func nearDuplicatePair(index int) (string, string) {
	var paragraph strings.Builder
	for section := 0; section < 40; section++ {
		_, _ = fmt.Fprintf(&paragraph, "Section %03d documents PostgreSQL engineering change %03d for fixture %03d and migration evidence. ", section, section, index)
	}
	return paragraph.String() + "Original publication.", paragraph.String() + "Updated publication."
}
func qualitySlice(index int) (string, string) {
	languages := []string{"zh", "en", "cross_language", "opposite_expression"}
	sources := []string{"feed", "platform", "search", "discussion"}
	return languages[index%4], sources[(index/4)%4]
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
