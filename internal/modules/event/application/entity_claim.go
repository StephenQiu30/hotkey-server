package application

import (
	"context"
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type EntityClaimStore interface {
	SaveEntity(context.Context, domain.Entity) (domain.Entity, error)
	SaveClaim(context.Context, domain.Claim) (domain.Claim, error)
}

type ClaimService struct{ store EntityClaimStore }

func NewClaimService(store EntityClaimStore) *ClaimService { return &ClaimService{store: store} }

func (service *ClaimService) Save(ctx context.Context, claim domain.Claim) (domain.Claim, error) {
	if service == nil || service.store == nil {
		return domain.Claim{}, fmt.Errorf("entity claim store is required")
	}
	if err := claim.Validate(); err != nil {
		return domain.Claim{}, err
	}
	if len(claim.Evidence) == 0 {
		return domain.Claim{}, fmt.Errorf("claim requires evidence")
	}
	// The PostgreSQL adapter re-checks active event membership inside its
	// transaction; callers cannot bypass evidence validity by supplying IDs.
	return service.store.SaveClaim(ctx, claim)
}

func SaveVerifiedClaim(ctx context.Context, store EntityClaimStore, claim domain.Claim, activeContentIDs map[int64]bool) (domain.Claim, error) {
	if store == nil {
		return domain.Claim{}, fmt.Errorf("entity claim store is required")
	}
	if err := claim.Validate(); err != nil {
		return domain.Claim{}, err
	}
	if len(claim.Evidence) == 0 {
		return domain.Claim{}, fmt.Errorf("claim requires evidence")
	}
	for _, evidence := range claim.Evidence {
		if !activeContentIDs[evidence.ContentID] {
			return domain.Claim{}, fmt.Errorf("claim evidence is inactive")
		}
	}
	return store.SaveClaim(ctx, claim)
}
