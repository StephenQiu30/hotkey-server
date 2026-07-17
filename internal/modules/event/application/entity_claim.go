package application

import (
	"context"
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type EntityClaimStore interface {
	SaveEntity(context.Context, domain.Entity) (domain.Entity, error)
	SaveEntityAlias(context.Context, domain.EntityAlias) (domain.EntityAlias, error)
	SaveEventEntity(context.Context, domain.EventEntity) (domain.EventEntity, error)
	SaveEntityRelation(context.Context, domain.EntityRelation) (domain.EntityRelation, error)
	SaveClaim(context.Context, domain.Claim) (domain.Claim, error)
}

type EntityService struct{ store EntityClaimStore }

func NewEntityService(store EntityClaimStore) *EntityService { return &EntityService{store: store} }

func (service *EntityService) Save(ctx context.Context, entity domain.Entity) (domain.Entity, error) {
	if service == nil || service.store == nil {
		return domain.Entity{}, fmt.Errorf("entity store is required")
	}
	if err := entity.Validate(); err != nil {
		return domain.Entity{}, err
	}
	return service.store.SaveEntity(ctx, entity)
}

func (service *EntityService) SaveAlias(ctx context.Context, alias domain.EntityAlias) (domain.EntityAlias, error) {
	if service == nil || service.store == nil {
		return domain.EntityAlias{}, fmt.Errorf("entity store is required")
	}
	if err := alias.Validate(); err != nil {
		return domain.EntityAlias{}, err
	}
	return service.store.SaveEntityAlias(ctx, alias)
}

func (service *EntityService) SaveEventEntity(ctx context.Context, entity domain.EventEntity) (domain.EventEntity, error) {
	if service == nil || service.store == nil {
		return domain.EventEntity{}, fmt.Errorf("entity store is required")
	}
	if err := entity.Validate(); err != nil {
		return domain.EventEntity{}, err
	}
	return service.store.SaveEventEntity(ctx, entity)
}

func (service *EntityService) SaveRelation(ctx context.Context, relation domain.EntityRelation) (domain.EntityRelation, error) {
	if service == nil || service.store == nil {
		return domain.EntityRelation{}, fmt.Errorf("entity store is required")
	}
	if err := relation.Validate(); err != nil {
		return domain.EntityRelation{}, err
	}
	return service.store.SaveEntityRelation(ctx, relation)
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
