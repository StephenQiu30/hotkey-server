package application

import (
	"context"
	"math"
	"strings"
)

const (
	// These two versions freeze one transparent rank-only probability model.
	// Raw channel scores are deliberately excluded because their scales are not
	// comparable. Activation remains a separate, evaluation-gated operation.
	CanonicalDocumentMatchRerankerVersion    = "rank-signal-logit-v1"
	CanonicalDocumentMatchCalibrationVersion = "time-split-platt-v1"
)

type RankSignalDocumentMatchReranker struct{}

var _ DocumentMatchReranker = (*RankSignalDocumentMatchReranker)(nil)

func NewRankSignalDocumentMatchReranker() *RankSignalDocumentMatchReranker {
	return &RankSignalDocumentMatchReranker{}
}

func (*RankSignalDocumentMatchReranker) RerankDocumentMatches(ctx context.Context, query RerankDocumentMatchesQuery) ([]RerankedDocumentMatchDTO, error) {
	if err := validateRankSignalRerankQuery(query); err != nil {
		return nil, err
	}
	result := make([]RerankedDocumentMatchDTO, 0, len(query.Candidates))
	for _, candidate := range query.Candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		linear := -4.0
		channels := make(map[string]struct{}, len(candidate.Signals))
		for _, signal := range candidate.Signals {
			quality := 1 - float64(signal.Rank-1)/100
			switch signal.Channel {
			case "lexical":
				linear += 1.7 * quality
			case "semantic":
				linear += 2.0 * quality
			case "structured":
				linear += 1.7 * quality
			}
			channels[signal.Channel] = struct{}{}
		}
		if len(channels) > 1 {
			linear += .6 * float64(len(channels)-1)
		}
		probability := logisticProbability(query.CalibrationSlope*linear + query.CalibrationIntercept)
		reasons := []string{"rank_signal_probability"}
		switch len(channels) {
		case 1:
			reasons = append(reasons, "single_channel_only")
		case 2:
			reasons = append(reasons, "two_channel_agreement")
		case 3:
			reasons = append(reasons, "three_channel_agreement")
		}
		reasons, err := sortedDocumentMatchReasons(reasons)
		if err != nil {
			return nil, err
		}
		result = append(result, RerankedDocumentMatchDTO{
			DocumentVersionID: candidate.DocumentVersionID, RelevanceProbability: probability, ReasonCodes: reasons,
		})
	}
	return result, nil
}

func validateRankSignalRerankQuery(query RerankDocumentMatchesQuery) error {
	if query.MonitorID <= 0 || query.MonitorVersionID <= 0 || query.CompiledProfileID <= 0 || query.RelevanceProfileID <= 0 ||
		query.MatchingAlgorithmVersion != HybridRecallMatchingAlgorithmVersion ||
		query.RerankerVersion != CanonicalDocumentMatchRerankerVersion ||
		!validDocumentMatchCalibrationProfileVersion(query.CalibrationVersion) || len(query.Candidates) > FusedRecallLimit {
		return ErrInvalidDocumentMatchContract
	}
	if math.IsNaN(query.CalibrationSlope) || math.IsInf(query.CalibrationSlope, 0) || query.CalibrationSlope <= 0 || query.CalibrationSlope > 100 ||
		math.IsNaN(query.CalibrationIntercept) || math.IsInf(query.CalibrationIntercept, 0) || math.Abs(query.CalibrationIntercept) > 100 {
		return ErrInvalidDocumentMatchContract
	}
	seenCandidates := make(map[int64]struct{}, len(query.Candidates))
	for _, candidate := range query.Candidates {
		if candidate.DocumentVersionID <= 0 || len(candidate.Signals) == 0 || len(candidate.Signals) > 3 ||
			math.IsNaN(candidate.RRFScore) || math.IsInf(candidate.RRFScore, 0) || candidate.RRFScore < 0 {
			return ErrInvalidDocumentMatchContract
		}
		if _, duplicate := seenCandidates[candidate.DocumentVersionID]; duplicate {
			return ErrInvalidDocumentMatchContract
		}
		seenCandidates[candidate.DocumentVersionID] = struct{}{}
		seenChannels := make(map[string]struct{}, len(candidate.Signals))
		for _, signal := range candidate.Signals {
			if signal.Rank < 1 || signal.Rank > 100 || math.IsNaN(signal.RawScore) || math.IsInf(signal.RawScore, 0) ||
				!validDocumentMatchSignalVersion(signal) {
				return ErrInvalidDocumentMatchContract
			}
			if _, duplicate := seenChannels[signal.Channel]; duplicate {
				return ErrInvalidDocumentMatchContract
			}
			seenChannels[signal.Channel] = struct{}{}
		}
	}
	return nil
}

func validDocumentMatchCalibrationProfileVersion(value string) bool {
	return value == CanonicalDocumentMatchCalibrationVersion ||
		strings.HasPrefix(value, CanonicalDocumentMatchCalibrationVersion+":") && semanticVersionPattern.MatchString(value)
}

func logisticProbability(value float64) float64 {
	if value >= 0 {
		exponential := math.Exp(-value)
		return 1 / (1 + exponential)
	}
	exponential := math.Exp(value)
	return exponential / (1 + exponential)
}

func validDocumentMatchSignalVersion(signal RecallSignalDTO) bool {
	switch signal.Channel {
	case "lexical":
		return signal.AlgorithmVersion == LexicalRecallAlgorithmVersion
	case "semantic":
		return signal.AlgorithmVersion == SemanticRecallAlgorithmVersion
	case "structured":
		return signal.AlgorithmVersion == StructuredRecallAlgorithmVersion
	default:
		return false
	}
}
