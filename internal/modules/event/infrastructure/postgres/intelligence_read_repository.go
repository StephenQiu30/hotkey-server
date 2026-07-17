package postgres

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var _ application.EventIntelligenceReadRepository = (*Repository)(nil)

func (repository *Repository) ReadEventIntelligence(ctx context.Context, eventID int64) (application.EventIntelligenceReadResult, error) {
	if !repository.available() || eventID <= 0 {
		return application.EventIntelligenceReadResult{}, sharedrepository.ErrUnavailable
	}
	if _, err := repository.Get(ctx, eventID); err != nil {
		return application.EventIntelligenceReadResult{}, err
	}
	var query rowsQuery = repository.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		query = transaction.SQL
	}
	entities, err := readEventIntelligenceEntities(ctx, query, eventID)
	if err != nil {
		return application.EventIntelligenceReadResult{}, err
	}
	claims, err := readEventIntelligenceClaims(ctx, query, eventID)
	if err != nil {
		return application.EventIntelligenceReadResult{}, err
	}
	return application.EventIntelligenceReadResult{EventID: eventID, Entities: entities, Claims: claims}, nil
}

func readEventIntelligenceEntities(ctx context.Context, query rowsQuery, eventID int64) ([]application.EventIntelligenceEntity, error) {
	rows, err := query.QueryContext(ctx, `
SELECT relation.id, relation.version, relation.event_id, relation.entity_id, relation.role, relation.confidence, relation.origin, relation.confirmed,
       entity.id, entity.version, entity.entity_key, entity.entity_type, entity.canonical_name, entity.description, entity.manual_locked
FROM event_entities relation
JOIN entities entity ON entity.id = relation.entity_id
WHERE relation.event_id = $1 AND entity.deleted_at IS NULL
ORDER BY relation.id ASC`, eventID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]application.EventIntelligenceEntity, 0)
	for rows.Next() {
		var item application.EventIntelligenceEntity
		if err := rows.Scan(&item.EventEntity.ID, &item.EventEntity.Version, &item.EventEntity.EventID, &item.EventEntity.EntityID, &item.EventEntity.Role, &item.EventEntity.Confidence, &item.EventEntity.Origin, &item.EventEntity.Confirmed,
			&item.Entity.ID, &item.Entity.Version, &item.Entity.Key, &item.Entity.Type, &item.Entity.Name, &item.Entity.Description, &item.Entity.ManualLocked); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return items, nil
}

func readEventIntelligenceClaims(ctx context.Context, query rowsQuery, eventID int64) ([]domain.Claim, error) {
	rows, err := query.QueryContext(ctx, `
SELECT id, version, event_id, normalized_claim, claim_hash, status, confidence, manual_locked
FROM event_claims WHERE event_id = $1 ORDER BY id ASC`, eventID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	claims := make([]domain.Claim, 0)
	for rows.Next() {
		var item domain.Claim
		if err := rows.Scan(&item.ID, &item.Version, &item.EventID, &item.NormalizedClaim, &item.ClaimHash, &item.Status, &item.Confidence, &item.ManualLocked); err != nil {
			rows.Close()
			return nil, sharedrepository.MapError(err)
		}
		claims = append(claims, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, sharedrepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	for index := range claims {
		evidence, err := readClaimEvidence(ctx, query, claims[index].ID)
		if err != nil {
			return nil, err
		}
		claims[index].Evidence = evidence
	}
	return claims, nil
}

func readClaimEvidence(ctx context.Context, query rowsQuery, claimID int64) ([]domain.ClaimEvidence, error) {
	rows, err := query.QueryContext(ctx, `
SELECT id, version, claim_id, content_id, evidence_locator, short_excerpt, stance, confidence
FROM claim_evidences WHERE claim_id = $1 ORDER BY id ASC`, claimID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	evidence := make([]domain.ClaimEvidence, 0)
	for rows.Next() {
		var item domain.ClaimEvidence
		if err := rows.Scan(&item.ID, &item.Version, &item.ClaimID, &item.ContentID, &item.Locator, &item.Excerpt, &item.Stance, &item.Confidence); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		evidence = append(evidence, item)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return evidence, nil
}
