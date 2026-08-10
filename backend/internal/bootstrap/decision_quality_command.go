package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedclock "github.com/StephenQiu30/hotkey-server/backend/internal/shared/clock"
)

const maximumDecisionQualityDatasetBytes = 64 << 20

type decisionQualityDatasetRequest struct {
	DatasetVersion                 string                                         `json:"dataset_version"`
	AnnotationProtocolVersion      string                                         `json:"annotation_protocol_version"`
	AnnotationGuidelineSHA256      string                                         `json:"annotation_guideline_sha256"`
	SplitStrategyVersion           string                                         `json:"split_strategy_version"`
	FamilyIsolationSHA256          string                                         `json:"family_isolation_sha256"`
	EventIsolationSHA256           string                                         `json:"event_isolation_sha256"`
	AnnotatorCount                 int                                            `json:"annotator_count"`
	AgreementMetric                string                                         `json:"agreement_metric"`
	AgreementScore                 float64                                        `json:"agreement_score"`
	TimeBoundary                   time.Time                                      `json:"time_boundary"`
	ContentFamilyProfileVersion    string                                         `json:"content_family_profile_version"`
	MicroEventProfileVersion       string                                         `json:"micro_event_profile_version"`
	EvidenceLocatorProfileVersion  string                                         `json:"evidence_locator_profile_version"`
	EvidenceRelationProfileVersion string                                         `json:"evidence_relation_profile_version"`
	HotspotProfileVersion          string                                         `json:"hotspot_profile_version"`
	DuplicateSamples               []decisionQualityDuplicateSampleRequest        `json:"duplicate_samples"`
	MicroEventSamples              []decisionQualityMicroEventSampleRequest       `json:"micro_event_samples"`
	EvidenceLocatorSamples         []decisionQualityEvidenceLocatorSampleRequest  `json:"evidence_locator_samples"`
	EvidenceRelationSamples        []decisionQualityEvidenceRelationSampleRequest `json:"evidence_relation_samples"`
	HotspotSamples                 []decisionQualityHotspotSampleRequest          `json:"hotspot_samples"`
}

type decisionQualityDuplicateSampleRequest struct {
	SampleID         string    `json:"sample_id"`
	Language         string    `json:"language"`
	SourceType       string    `json:"source_type"`
	LeftFixtureText  string    `json:"left_fixture_text"`
	RightFixtureText string    `json:"right_fixture_text"`
	ObservedAt       time.Time `json:"observed_at"`
	Duplicate        bool      `json:"duplicate"`
	HardNegative     bool      `json:"hard_negative"`
}

type decisionQualityMicroEventSampleRequest struct {
	SampleID       string                                   `json:"sample_id"`
	Language       string                                   `json:"language"`
	SourceType     string                                   `json:"source_type"`
	EventSize      string                                   `json:"event_size"`
	ObservedAt     time.Time                                `json:"observed_at"`
	SameEvent      bool                                     `json:"same_event"`
	DenseAvailable bool                                     `json:"dense_available"`
	HardConflict   bool                                     `json:"hard_conflict"`
	Features       decisionQualityMicroEventFeaturesRequest `json:"features"`
}

type decisionQualityMicroEventFeaturesRequest struct {
	SparseSimilarity      float64 `json:"sparse_similarity"`
	DenseSimilarity       float64 `json:"dense_similarity"`
	EntityOverlap         float64 `json:"entity_overlap"`
	ActionOverlap         float64 `json:"action_overlap"`
	LocationConsistency   float64 `json:"location_consistency"`
	IdentifierConsistency float64 `json:"identifier_consistency"`
	TimeSimilarity        float64 `json:"time_similarity"`
	LineageRelation       float64 `json:"lineage_relation"`
}

type decisionQualityEvidenceLocatorSampleRequest struct {
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

type decisionQualityEvidenceRelationSampleRequest struct {
	SampleID          string    `json:"sample_id"`
	Language          string    `json:"language"`
	SourceType        string    `json:"source_type"`
	ExpectedRelation  string    `json:"expected_relation"`
	PredictedRelation string    `json:"predicted_relation"`
	ObservedAt        time.Time `json:"observed_at"`
}

type decisionQualityHotspotSampleRequest struct {
	SampleID              string                          `json:"sample_id"`
	Language              string                          `json:"language"`
	SourceType            string                          `json:"source_type"`
	ObservedAt            time.Time                       `json:"observed_at"`
	ExpectedHotspot       bool                            `json:"expected_hotspot"`
	DiscoveryDelaySeconds float64                         `json:"discovery_delay_seconds"`
	Threshold             float64                         `json:"threshold"`
	Input                 decisionQualityHeatInputRequest `json:"input"`
}

type decisionQualityHeatInputRequest struct {
	IndependentLineageRoots int      `json:"independent_lineage_roots"`
	ReportsInWindow         int      `json:"reports_in_window"`
	ReportsInPreviousWindow int      `json:"reports_in_previous_window"`
	ReportsInPriorWindow    int      `json:"reports_in_prior_window"`
	PublisherCoverage       int      `json:"publisher_coverage"`
	SourceTypeCoverage      int      `json:"source_type_coverage"`
	NormalizedEngagement    *float64 `json:"normalized_engagement"`
	AgeHours                float64  `json:"age_hours"`
}

type decisionQualityCommandResponse struct {
	Evaluation  operationsapplication.DecisionQualityEvaluationResult         `json:"evaluation"`
	Persistence *operationsapplication.PersistDecisionQualityEvaluationResult `json:"persistence,omitempty"`
}

func runDecisionQualityCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	if len(args) == 0 || args[0] != "evaluate" {
		return errors.New("quality command is required: expected evaluate")
	}
	flags := flag.NewFlagSet("hotkey quality evaluate", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	datasetPath := flags.String("dataset", "", "time-isolated decision quality dataset JSON")
	actorUserID := flags.Int64("actor-user-id", 0, "active administrator user ID used for an auditable evaluation")
	activate := flags.Bool("activate", false, "activate only profiles whose global and required-slice gates pass")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse quality evaluate flags: %w", err)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*datasetPath) == "" || output == nil || *actorUserID < 0 || *activate && *actorUserID <= 0 {
		return errors.New("quality evaluate requires --dataset")
	}
	dataset, err := readDecisionQualityDataset(*datasetPath)
	if err != nil {
		return err
	}
	response := decisionQualityCommandResponse{}
	if *actorUserID == 0 {
		response.Evaluation, err = operationsapplication.EvaluateDecisionQuality(dataset)
		if err != nil {
			return err
		}
	} else {
		if strings.TrimSpace(cfg.DatabaseURL) == "" {
			return errors.New("quality evaluate persistence requires the database URL")
		}
		runtime, openError := database.Open(ctx, cfg.DatabaseURL)
		if openError != nil {
			return openError
		}
		defer func() { _ = runtime.Close() }()
		service, serviceError := operationsapplication.NewDecisionQualityService(operationspostgres.NewDecisionQualityRepository(runtime), sharedclock.System{})
		if serviceError != nil {
			return serviceError
		}
		persistence := operationsapplication.PersistDecisionQualityEvaluationResult{}
		response.Evaluation, persistence, err = service.Evaluate(ctx, dataset, *actorUserID, *activate)
		if err != nil {
			return err
		}
		response.Persistence = &persistence
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(response)
}

func readDecisionQualityDataset(path string) (operationsapplication.DecisionQualityDatasetDTO, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return operationsapplication.DecisionQualityDatasetDTO{}, fmt.Errorf("open decision quality dataset: %w", err)
	}
	if len(encoded) > maximumDecisionQualityDatasetBytes {
		return operationsapplication.DecisionQualityDatasetDTO{}, errors.New("decision quality dataset exceeds 64 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	var request decisionQualityDatasetRequest
	if err := decoder.Decode(&request); err != nil {
		return operationsapplication.DecisionQualityDatasetDTO{}, fmt.Errorf("decode decision quality dataset: %w", err)
	}
	if err := ensureRelevanceDatasetEOF(decoder); err != nil {
		return operationsapplication.DecisionQualityDatasetDTO{}, err
	}
	digest := sha256.Sum256(encoded)
	result := operationsapplication.DecisionQualityDatasetDTO{
		DatasetVersion: request.DatasetVersion, DatasetSHA256: hex.EncodeToString(digest[:]),
		AnnotationProtocolVersion: request.AnnotationProtocolVersion, AnnotationGuidelineSHA256: request.AnnotationGuidelineSHA256,
		SplitStrategyVersion: request.SplitStrategyVersion, FamilyIsolationSHA256: request.FamilyIsolationSHA256,
		EventIsolationSHA256: request.EventIsolationSHA256, AnnotatorCount: request.AnnotatorCount,
		AgreementMetric: request.AgreementMetric, AgreementScore: request.AgreementScore, TimeBoundary: request.TimeBoundary,
		ContentFamilyProfileVersion: request.ContentFamilyProfileVersion, MicroEventProfileVersion: request.MicroEventProfileVersion,
		EvidenceLocatorProfileVersion: request.EvidenceLocatorProfileVersion, EvidenceRelationProfileVersion: request.EvidenceRelationProfileVersion,
		HotspotProfileVersion:   request.HotspotProfileVersion,
		DuplicateSamples:        make([]operationsapplication.DuplicateQualitySampleDTO, len(request.DuplicateSamples)),
		MicroEventSamples:       make([]operationsapplication.MicroEventQualitySampleDTO, len(request.MicroEventSamples)),
		EvidenceLocatorSamples:  make([]operationsapplication.EvidenceLocatorQualitySampleDTO, len(request.EvidenceLocatorSamples)),
		EvidenceRelationSamples: make([]operationsapplication.EvidenceRelationQualitySampleDTO, len(request.EvidenceRelationSamples)),
		HotspotSamples:          make([]operationsapplication.HotspotQualitySampleDTO, len(request.HotspotSamples)),
	}
	for index, sample := range request.DuplicateSamples {
		result.DuplicateSamples[index] = operationsapplication.DuplicateQualitySampleDTO{SampleID: sample.SampleID, Language: sample.Language, SourceType: sample.SourceType, LeftFixtureText: sample.LeftFixtureText, RightFixtureText: sample.RightFixtureText, ObservedAt: sample.ObservedAt, Duplicate: sample.Duplicate, HardNegative: sample.HardNegative}
	}
	for index, sample := range request.MicroEventSamples {
		result.MicroEventSamples[index] = operationsapplication.MicroEventQualitySampleDTO{SampleID: sample.SampleID, Language: sample.Language, SourceType: sample.SourceType, EventSize: sample.EventSize, ObservedAt: sample.ObservedAt, SameEvent: sample.SameEvent, DenseAvailable: sample.DenseAvailable, HardConflict: sample.HardConflict,
			Features: operationsapplication.MicroEventQualityFeaturesDTO{SparseSimilarity: sample.Features.SparseSimilarity,
				DenseSimilarity: sample.Features.DenseSimilarity, EntityOverlap: sample.Features.EntityOverlap,
				ActionOverlap: sample.Features.ActionOverlap, LocationConsistency: sample.Features.LocationConsistency,
				IdentifierConsistency: sample.Features.IdentifierConsistency, TimeSimilarity: sample.Features.TimeSimilarity,
				LineageRelation: sample.Features.LineageRelation}}
	}
	for index, sample := range request.EvidenceLocatorSamples {
		result.EvidenceLocatorSamples[index] = operationsapplication.EvidenceLocatorQualitySampleDTO{SampleID: sample.SampleID, Language: sample.Language, SourceType: sample.SourceType, SyntheticPlaintext: sample.SyntheticPlaintext, ExactQuote: sample.ExactQuote, Prefix: sample.Prefix, Suffix: sample.Suffix, PlaintextSHA256: sample.PlaintextSHA256, UTF8ByteStart: sample.UTF8ByteStart, UTF8ByteEnd: sample.UTF8ByteEnd, ObservedAt: sample.ObservedAt, CitationFieldsComplete: sample.CitationFieldsComplete}
	}
	for index, sample := range request.EvidenceRelationSamples {
		result.EvidenceRelationSamples[index] = operationsapplication.EvidenceRelationQualitySampleDTO{SampleID: sample.SampleID, Language: sample.Language, SourceType: sample.SourceType, ExpectedRelation: sample.ExpectedRelation, PredictedRelation: sample.PredictedRelation, ObservedAt: sample.ObservedAt}
	}
	for index, sample := range request.HotspotSamples {
		result.HotspotSamples[index] = operationsapplication.HotspotQualitySampleDTO{SampleID: sample.SampleID, Language: sample.Language, SourceType: sample.SourceType, ObservedAt: sample.ObservedAt, ExpectedHotspot: sample.ExpectedHotspot, DiscoveryDelaySeconds: sample.DiscoveryDelaySeconds, Threshold: sample.Threshold,
			Input: operationsapplication.EventHeatQualityInputDTO{IndependentLineageRoots: sample.Input.IndependentLineageRoots,
				ReportsInWindow: sample.Input.ReportsInWindow, ReportsInPreviousWindow: sample.Input.ReportsInPreviousWindow,
				ReportsInPriorWindow: sample.Input.ReportsInPriorWindow, PublisherCoverage: sample.Input.PublisherCoverage,
				SourceTypeCoverage: sample.Input.SourceTypeCoverage, NormalizedEngagement: sample.Input.NormalizedEngagement,
				AgeHours: sample.Input.AgeHours}}
	}
	return result, nil
}
