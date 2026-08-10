package http

import (
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

type DocumentMatchSignalResponseDTO struct {
	Channel          string  `json:"channel" enums:"lexical,semantic,structured"`
	Rank             int     `json:"rank"`
	RawScore         float64 `json:"raw_score"`
	AlgorithmVersion string  `json:"algorithm_version"`
}

type DocumentMatchResponseDTO struct {
	MatchDecisionID          int64                            `json:"match_decision_id"`
	MonitorID                int64                            `json:"monitor_id"`
	MonitorVersionID         int64                            `json:"monitor_version_id"`
	CompiledProfileID        int64                            `json:"compiled_profile_id"`
	DocumentVersionID        int64                            `json:"document_version_id"`
	RelevanceProfileID       int64                            `json:"relevance_profile_id"`
	MatchingAlgorithmVersion string                           `json:"matching_algorithm_version"`
	RerankerVersion          string                           `json:"reranker_version"`
	CalibrationVersion       string                           `json:"calibration_version"`
	RRFScore                 float64                          `json:"rrf_score"`
	RelevanceProbability     *float64                         `json:"relevance_probability" extensions:"x-nullable"`
	AutomaticDecision        string                           `json:"automatic_decision" enums:"accepted,review,rejected"`
	EffectiveDecision        string                           `json:"effective_decision" enums:"accepted,review,rejected"`
	Degraded                 bool                             `json:"degraded"`
	ReasonCodes              []string                         `json:"reason_codes"`
	Signals                  []DocumentMatchSignalResponseDTO `json:"signals"`
	ResourceVersion          int64                            `json:"resource_version"`
	DecidedAt                time.Time                        `json:"decided_at"`
}

type DocumentMatchPageResponseDTO struct {
	Items      []DocumentMatchResponseDTO `json:"items"`
	NextCursor string                     `json:"next_cursor"`
}

type OverrideDocumentMatchRequestDTO struct {
	Decision   string `json:"decision" binding:"required" enums:"accepted,rejected"`
	ReasonCode string `json:"reason_code" binding:"required"`
	Note       string `json:"note"`
}

type OverrideDocumentMatchResponseDTO struct {
	OverrideID                int64     `json:"override_id"`
	MatchDecisionID           int64     `json:"match_decision_id"`
	MonitorID                 int64     `json:"monitor_id"`
	MonitorVersionID          int64     `json:"monitor_version_id"`
	DocumentVersionID         int64     `json:"document_version_id"`
	PreviousEffectiveDecision string    `json:"previous_effective_decision" enums:"accepted,review,rejected"`
	Decision                  string    `json:"decision" enums:"accepted,rejected"`
	ReasonCode                string    `json:"reason_code"`
	Note                      string    `json:"note"`
	ActorUserID               int64     `json:"actor_user_id"`
	ResourceVersion           int64     `json:"resource_version"`
	CreatedAt                 time.Time `json:"created_at"`
	Reused                    bool      `json:"reused"`
}

func documentMatchPageResponseDTO(value ingestionapplication.DocumentMatchPageResult) DocumentMatchPageResponseDTO {
	items := make([]DocumentMatchResponseDTO, len(value.Items))
	for index, item := range value.Items {
		items[index] = documentMatchResponseDTO(item)
	}
	return DocumentMatchPageResponseDTO{Items: items, NextCursor: value.NextCursor}
}

func documentMatchResponseDTO(value ingestionapplication.DocumentMatchListItemDTO) DocumentMatchResponseDTO {
	signals := make([]DocumentMatchSignalResponseDTO, len(value.Automatic.Signals))
	for index, signal := range value.Automatic.Signals {
		signals[index] = DocumentMatchSignalResponseDTO{
			Channel: signal.Channel, Rank: signal.Rank, RawScore: signal.RawScore, AlgorithmVersion: signal.AlgorithmVersion,
		}
	}
	return DocumentMatchResponseDTO{
		MatchDecisionID: value.Automatic.ID, MonitorID: value.Automatic.MonitorID,
		MonitorVersionID: value.Automatic.MonitorVersionID, CompiledProfileID: value.Automatic.CompiledProfileID,
		DocumentVersionID: value.Automatic.DocumentVersionID, RelevanceProfileID: value.Automatic.RelevanceProfileID,
		MatchingAlgorithmVersion: value.Automatic.MatchingAlgorithmVersion, RerankerVersion: value.Automatic.RerankerVersion,
		CalibrationVersion: value.Automatic.CalibrationVersion, RRFScore: value.Automatic.RRFScore,
		RelevanceProbability: value.Automatic.RelevanceProbability, AutomaticDecision: value.Automatic.Decision,
		EffectiveDecision: value.EffectiveDecision, Degraded: value.Automatic.Degraded,
		ReasonCodes: append([]string(nil), value.Automatic.ReasonCodes...), Signals: signals,
		ResourceVersion: value.OverrideSequence, DecidedAt: value.Automatic.CreatedAt,
	}
}

func overrideDocumentMatchResponseDTO(value ingestionapplication.OverrideDocumentMatchResult) OverrideDocumentMatchResponseDTO {
	override := value.Override
	return OverrideDocumentMatchResponseDTO{
		OverrideID: override.ID, MatchDecisionID: override.MatchDecisionID, MonitorID: override.MonitorID,
		MonitorVersionID: override.MonitorVersionID, DocumentVersionID: override.DocumentVersionID,
		PreviousEffectiveDecision: override.PreviousEffectiveDecision, Decision: override.Decision,
		ReasonCode: override.ReasonCode, Note: override.Note, ActorUserID: override.ActorUserID,
		ResourceVersion: override.Sequence, CreatedAt: override.CreatedAt, Reused: value.Reused,
	}
}
