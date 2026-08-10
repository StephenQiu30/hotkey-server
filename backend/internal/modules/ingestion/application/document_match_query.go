package application

import (
	"context"
	"fmt"
	"math"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

const DefaultDocumentMatchPageSize = 50
const MaximumDocumentMatchPageSize = 100

type ListDocumentMatchesQuery struct {
	ActorUserID       int64
	MonitorID         int64
	EffectiveDecision string
	Cursor            string
	Limit             int
}

type DocumentMatchListItemDTO struct {
	Automatic         DocumentMatchDecisionDTO
	EffectiveDecision string
	OverrideSequence  int64
}

type DocumentMatchPageResult struct {
	Items      []DocumentMatchListItemDTO
	NextCursor string
}

type DocumentMatchReader interface {
	ListDocumentMatches(context.Context, ListDocumentMatchesQuery) (DocumentMatchPageResult, error)
}

type DocumentMatchQueryService struct{ reader DocumentMatchReader }

func NewDocumentMatchQueryService(reader DocumentMatchReader) (*DocumentMatchQueryService, error) {
	if reader == nil {
		return nil, fmt.Errorf("%w: document match reader is required", ErrInvalidDocumentMatchContract)
	}
	return &DocumentMatchQueryService{reader: reader}, nil
}

func (service *DocumentMatchQueryService) List(ctx context.Context, query ListDocumentMatchesQuery) (DocumentMatchPageResult, error) {
	if service == nil || service.reader == nil || query.ActorUserID <= 0 || query.MonitorID <= 0 ||
		!validDocumentMatchDecisionFilter(query.EffectiveDecision) || query.Limit < 0 || query.Limit > MaximumDocumentMatchPageSize {
		return DocumentMatchPageResult{}, ErrInvalidDocumentMatchContract
	}
	if query.Limit == 0 {
		query.Limit = DefaultDocumentMatchPageSize
	}
	result, err := service.reader.ListDocumentMatches(ctx, query)
	if err != nil {
		return DocumentMatchPageResult{}, err
	}
	if len(result.Items) > query.Limit {
		return DocumentMatchPageResult{}, ErrInvalidDocumentMatchContract
	}
	for _, item := range result.Items {
		if !validDocumentMatchListItem(item, query.MonitorID, query.EffectiveDecision) {
			return DocumentMatchPageResult{}, ErrInvalidDocumentMatchContract
		}
	}
	result.Items = append([]DocumentMatchListItemDTO(nil), result.Items...)
	return result, nil
}

func validDocumentMatchDecisionFilter(value string) bool {
	return value == "" || value == "accepted" || value == "review" || value == "rejected"
}

func validDocumentMatchListItem(item DocumentMatchListItemDTO, monitorID int64, filter string) bool {
	decision := item.Automatic
	entity := ingestiondomain.DocumentMatchDecision{
		ID: decision.ID, MonitorID: decision.MonitorID, MonitorVersionID: decision.MonitorVersionID,
		CompiledProfileID: decision.CompiledProfileID, DocumentVersionID: decision.DocumentVersionID,
		RelevanceProfileID: decision.RelevanceProfileID, MatchingAlgorithmVersion: decision.MatchingAlgorithmVersion,
		RerankerVersion: decision.RerankerVersion, CalibrationVersion: decision.CalibrationVersion,
		InputHash: decision.InputHash, RRFScore: decision.RRFScore, RelevanceProbability: decision.RelevanceProbability,
		Decision: ingestiondomain.MatchDecision(decision.Decision), Degraded: decision.Degraded,
		ReasonCodes: decision.ReasonCodes, CreatedAt: decision.CreatedAt,
	}
	if entity.Validate() != nil || decision.MonitorID != monitorID || !validDocumentMatchDecisionFilter(item.EffectiveDecision) ||
		item.EffectiveDecision == "" || item.OverrideSequence < 0 || filter != "" && item.EffectiveDecision != filter {
		return false
	}
	if item.OverrideSequence == 0 && item.EffectiveDecision != decision.Decision {
		return false
	}
	seenChannels := make(map[string]struct{}, len(decision.Signals))
	for _, signal := range decision.Signals {
		if (signal.Channel != "lexical" && signal.Channel != "semantic" && signal.Channel != "structured") ||
			signal.Rank < 1 || signal.Rank > 100 || signal.AlgorithmVersion == "" ||
			math.IsNaN(signal.RawScore) || math.IsInf(signal.RawScore, 0) {
			return false
		}
		if _, duplicate := seenChannels[signal.Channel]; duplicate {
			return false
		}
		seenChannels[signal.Channel] = struct{}{}
	}
	return true
}
