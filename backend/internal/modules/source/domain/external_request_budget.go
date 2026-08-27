package domain

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

var resourceProfileVersionPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}-v[1-9][0-9]*$`)

// ExternalRequestBudgetReservation is the bounded identity used immediately
// before one physical provider request. It intentionally contains no URL,
// query, credential or response data.
type ExternalRequestBudgetReservation struct {
	SourceConnectionID     int64
	ResourceProfileVersion string
	DailyLimit             int64
	At                     time.Time
}

func (reservation ExternalRequestBudgetReservation) Validate() error {
	if reservation.SourceConnectionID <= 0 || len(reservation.ResourceProfileVersion) > 64 || !resourceProfileVersionPattern.MatchString(reservation.ResourceProfileVersion) ||
		reservation.DailyLimit < 1 || reservation.DailyLimit > 1_000_000 || reservation.At.IsZero() {
		return fmt.Errorf("external request budget reservation is invalid")
	}
	return nil
}

type ExternalRequestBudgetDecision struct {
	Allowed bool
	Used    int64
	ResetAt time.Time
}

func (decision ExternalRequestBudgetDecision) Validate(limit int64) error {
	if limit < 1 || decision.Used < 0 || decision.Used > limit || decision.ResetAt.IsZero() || (decision.Allowed && decision.Used < 1) {
		return fmt.Errorf("external request budget decision is invalid")
	}
	return nil
}

// ExternalRequestBudget is implemented by the Source-owned durable ledger.
// Connector adapters reserve once for every physical request, including
// redirects and retries, before any network side effect.
type ExternalRequestBudget interface {
	ReserveExternalRequest(context.Context, ExternalRequestBudgetReservation) (ExternalRequestBudgetDecision, error)
}
