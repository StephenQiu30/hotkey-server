package application

import (
	"context"
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type DecisionStore interface {
	SaveDecisions(context.Context, []domain.Decision) error
}

func PersistDecisions(ctx context.Context, store DecisionStore, decisions []domain.Decision) error {
	if store == nil || len(decisions) == 0 {
		return fmt.Errorf("decision store and decisions are required")
	}
	for _, decision := range decisions {
		if err := decision.Validate(); err != nil {
			return err
		}
	}
	return store.SaveDecisions(ctx, decisions)
}
