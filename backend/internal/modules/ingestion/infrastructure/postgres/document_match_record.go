package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

type relevanceDecisionProfileRecord struct {
	ID, Version, EvaluationRunID              int64
	MatchingAlgorithmVersion, RerankerVersion string
	CalibrationVersion, Status                string
	RejectThreshold, AcceptThreshold          float64
	CalibrationSlope, CalibrationIntercept    float64
}

func relevanceDecisionProfileDTO(record relevanceDecisionProfileRecord) ingestionapplication.RelevanceDecisionProfileDTO {
	return ingestionapplication.RelevanceDecisionProfileDTO{
		ID: record.ID, Version: record.Version, EvaluationRunID: record.EvaluationRunID, MatchingAlgorithmVersion: record.MatchingAlgorithmVersion,
		RerankerVersion: record.RerankerVersion, CalibrationVersion: record.CalibrationVersion,
		Status: record.Status, RejectThreshold: record.RejectThreshold, AcceptThreshold: record.AcceptThreshold,
		CalibrationSlope: record.CalibrationSlope, CalibrationIntercept: record.CalibrationIntercept,
	}
}

type documentMatchDecisionRecord struct {
	ID, MonitorID, MonitorVersionID, CompiledProfileID int64
	DocumentVersionID, RelevanceProfileID              int64
	MatchingAlgorithmVersion, RerankerVersion          string
	CalibrationVersion, InputHash, Decision            string
	RRFScore                                           float64
	RelevanceProbability                               sql.NullFloat64
	Degraded                                           bool
	ReasonCodesJSON                                    string
	DecidedAt                                          time.Time
}

type documentMatchSignalRecord struct {
	Channel          string  `json:"channel"`
	Rank             int     `json:"rank"`
	RawScore         float64 `json:"raw_score"`
	AlgorithmVersion string  `json:"algorithm_version"`
}

func documentMatchSignalsFromJSON(value string) ([]ingestionapplication.DocumentMatchSignalDTO, error) {
	records := []documentMatchSignalRecord{}
	if err := json.Unmarshal([]byte(value), &records); err != nil {
		return nil, fmt.Errorf("decode document match signals: %w", err)
	}
	result := make([]ingestionapplication.DocumentMatchSignalDTO, len(records))
	for index, record := range records {
		result[index] = ingestionapplication.DocumentMatchSignalDTO{
			Channel: record.Channel, Rank: record.Rank, RawScore: record.RawScore, AlgorithmVersion: record.AlgorithmVersion,
		}
	}
	return result, nil
}

func (record documentMatchDecisionRecord) dto(signals []ingestionapplication.DocumentMatchSignalDTO) (ingestionapplication.DocumentMatchDecisionDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal([]byte(record.ReasonCodesJSON), &reasons); err != nil {
		return ingestionapplication.DocumentMatchDecisionDTO{}, fmt.Errorf("decode document match reasons: %w", err)
	}
	var probability *float64
	if record.RelevanceProbability.Valid {
		value := record.RelevanceProbability.Float64
		probability = &value
	}
	return ingestionapplication.DocumentMatchDecisionDTO{
		ID: record.ID, MonitorID: record.MonitorID, MonitorVersionID: record.MonitorVersionID,
		CompiledProfileID: record.CompiledProfileID, DocumentVersionID: record.DocumentVersionID,
		RelevanceProfileID: record.RelevanceProfileID, MatchingAlgorithmVersion: record.MatchingAlgorithmVersion,
		RerankerVersion: record.RerankerVersion, CalibrationVersion: record.CalibrationVersion,
		InputHash: record.InputHash, RRFScore: record.RRFScore, RelevanceProbability: probability,
		Decision: record.Decision, Degraded: record.Degraded, ReasonCodes: reasons,
		Signals: append([]ingestionapplication.DocumentMatchSignalDTO(nil), signals...), CreatedAt: record.DecidedAt.UTC(),
	}, nil
}

type documentMatchOverrideRecord struct {
	ID, MatchDecisionID, Sequence, MonitorID, MonitorVersionID, DocumentVersionID int64
	Decision, PreviousEffectiveDecision, ReasonCode, Note                         string
	ActorUserID                                                                   int64
	CreatedAt                                                                     time.Time
}

func documentMatchOverrideDTO(record documentMatchOverrideRecord) ingestionapplication.DocumentMatchOverrideDTO {
	return ingestionapplication.DocumentMatchOverrideDTO{
		ID: record.ID, MatchDecisionID: record.MatchDecisionID, Sequence: record.Sequence,
		MonitorID: record.MonitorID, MonitorVersionID: record.MonitorVersionID,
		DocumentVersionID: record.DocumentVersionID, Decision: record.Decision,
		PreviousEffectiveDecision: record.PreviousEffectiveDecision, ReasonCode: record.ReasonCode,
		Note: record.Note, ActorUserID: record.ActorUserID, CreatedAt: record.CreatedAt.UTC(),
	}
}
