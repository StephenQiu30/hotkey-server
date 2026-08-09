package postgres

import (
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

type compiledRecallProfileRecord struct {
	ID, MonitorID, ConfigVersionID, MonitorVersionID int64
	PreviewRunID, DraftID, DraftResourceVersion      int64
	Purpose                                          string
	MatchingAlgorithmVersion                         string
	LexicalAlgorithmVersion                          string
	SemanticAlgorithmVersion                         string
	StructuredAlgorithmVersion                       string
	SearchNormalizationProfileVersion                string
	SemanticState, SemanticUnavailableReason         string
	EmbeddingProfileID, EmbeddingProfileVersion      int64
	ModelVersion                                     string
	QueryVector                                      []float32
}

type compiledRecallClauseRecord struct {
	Operator, Field, Value, Origin string
}

type compiledRecallEntityRecord struct {
	ID          int64
	CanonicalID string
	Aliases     []string
}

func compiledRecallProfileDTO(record compiledRecallProfileRecord, clauses []compiledRecallClauseRecord, entities []compiledRecallEntityRecord) (ingestionapplication.ReadyRecallProfileDTO, error) {
	if record.ID <= 0 || (record.SemanticState == ingestionapplication.SemanticRecallStateReady && len(record.QueryVector) != 1024) ||
		(record.SemanticState == ingestionapplication.SemanticRecallStateUnavailable && (record.SemanticUnavailableReason == "" || len(record.QueryVector) != 0)) {
		return ingestionapplication.ReadyRecallProfileDTO{}, fmt.Errorf("compiled recall profile record is incomplete")
	}
	clauseDTOs := make([]ingestionapplication.RecallClauseDTO, 0, len(clauses))
	for _, clause := range clauses {
		clauseDTOs = append(clauseDTOs, ingestionapplication.RecallClauseDTO{
			Operator: clause.Operator, Field: clause.Field, Value: clause.Value, Origin: clause.Origin,
		})
	}
	entityDTOs := make([]ingestionapplication.RecallEntityDTO, 0, len(entities))
	for _, entity := range entities {
		entityDTOs = append(entityDTOs, ingestionapplication.RecallEntityDTO{
			CanonicalID: entity.CanonicalID, Aliases: append([]string(nil), entity.Aliases...),
		})
	}
	result := ingestionapplication.ReadyRecallProfileDTO{
		MonitorID: record.MonitorID, Purpose: record.Purpose, ConfigVersionID: record.ConfigVersionID,
		MonitorVersionID: record.MonitorVersionID, PreviewRunID: record.PreviewRunID,
		DraftID: record.DraftID, DraftResourceVersion: record.DraftResourceVersion,
		CompiledProfileID: record.ID, MatchingAlgorithmVersion: record.MatchingAlgorithmVersion,
		LexicalAlgorithmVersion:           record.LexicalAlgorithmVersion,
		SemanticAlgorithmVersion:          record.SemanticAlgorithmVersion,
		StructuredAlgorithmVersion:        record.StructuredAlgorithmVersion,
		SearchNormalizationProfileVersion: record.SearchNormalizationProfileVersion,
		SemanticState:                     record.SemanticState,
		SemanticUnavailableReason:         record.SemanticUnavailableReason,
		Clauses:                           clauseDTOs, Entities: entityDTOs,
	}
	if record.SemanticState == ingestionapplication.SemanticRecallStateReady {
		result.Semantic = &ingestionapplication.SemanticRecallProfileDTO{
			EmbeddingProfileID:      record.EmbeddingProfileID,
			EmbeddingProfileVersion: record.EmbeddingProfileVersion,
			ModelVersion:            record.ModelVersion, QueryVector: append([]float32(nil), record.QueryVector...),
		}
	}
	return result, nil
}
