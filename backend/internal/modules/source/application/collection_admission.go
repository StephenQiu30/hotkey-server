package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

// CurrentCollectionFetchRightsQuery identifies the source endpoint whose
// current fetch permission must be evaluated before credentials, budget, or
// network access are possible.
type CurrentCollectionFetchRightsQuery struct {
	SourceConnectionID int64
	DecisionAt         time.Time
}

func (query CurrentCollectionFetchRightsQuery) Validate() error {
	if query.SourceConnectionID <= 0 || query.DecisionAt.IsZero() {
		return errors.New("collection fetch rights source and decision time are required")
	}
	return nil
}

type CurrentCollectionFetchRightsResult struct {
	Decision    domain.RightsState
	DecisionIDs []int64
	PolicyIDs   []int64
	EvaluatedAt time.Time
}

func (result CurrentCollectionFetchRightsResult) Validate(query CurrentCollectionFetchRightsQuery) error {
	if err := query.Validate(); err != nil || !result.Decision.Valid() || !result.EvaluatedAt.Equal(query.DecisionAt.UTC()) {
		return errors.New("collection fetch rights result is invalid")
	}
	if result.Decision == domain.RightsUnknown {
		if len(result.DecisionIDs) != 0 || len(result.PolicyIDs) != 0 {
			return errors.New("unknown collection fetch rights cannot carry receipts")
		}
		return nil
	}
	if len(result.DecisionIDs) == 0 || len(result.PolicyIDs) == 0 || !positiveSortedUnique(result.DecisionIDs) || !positiveSortedUnique(result.PolicyIDs) {
		return errors.New("collection fetch rights receipts are invalid")
	}
	return nil
}

func positiveSortedUnique(values []int64) bool {
	if len(values) == 0 || values[0] <= 0 {
		return false
	}
	for index := 1; index < len(values); index++ {
		if values[index] <= values[index-1] {
			return false
		}
	}
	return true
}

type CurrentCollectionFetchRightsReader interface {
	ResolveCurrentFetch(context.Context, CurrentCollectionFetchRightsQuery) (CurrentCollectionFetchRightsResult, error)
}

type CollectionAdmissionAuthorizer interface {
	AuthorizeCollection(context.Context, domain.SourceConnection) error
}

type CollectionAdmissionDependencies struct {
	Rights CurrentCollectionFetchRightsReader
	Clock  Clock
}

type CollectionAdmissionGate struct {
	rights CurrentCollectionFetchRightsReader
	clock  Clock
}

func NewCollectionAdmissionGate(dependencies CollectionAdmissionDependencies) (*CollectionAdmissionGate, error) {
	if dependencies.Rights == nil {
		return nil, errors.New("collection fetch rights reader is required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = wallClock{}
	}
	return &CollectionAdmissionGate{rights: dependencies.Rights, clock: dependencies.Clock}, nil
}

func (gate *CollectionAdmissionGate) AuthorizeCollection(ctx context.Context, connection domain.SourceConnection) error {
	if gate == nil || gate.rights == nil || gate.clock == nil {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("collection admission is unavailable"))
	}
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.ID <= 0 || normalized.Deleted || !normalized.Enabled || normalized.HealthStatus == domain.HealthStatusUnavailable {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("collection source capability is unavailable"))
	}
	decisionAt := gate.clock.Now().UTC()
	query := CurrentCollectionFetchRightsQuery{SourceConnectionID: normalized.ID, DecisionAt: decisionAt}
	if err := query.Validate(); err != nil {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("collection fetch rights input is invalid"))
	}
	result, err := gate.rights.ResolveCurrentFetch(ctx, query)
	if err != nil {
		return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New("collection fetch rights are unavailable"))
	}
	if err := result.Validate(query); err != nil {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, fmt.Errorf("collection fetch rights result is invalid: %w", err))
	}
	if result.Decision != domain.RightsAllow {
		return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("source fetch rights are not permitted"))
	}
	return nil
}
