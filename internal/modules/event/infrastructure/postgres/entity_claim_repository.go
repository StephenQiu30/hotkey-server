package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func (repository *Repository) SaveEntity(ctx context.Context, entity domain.Entity) (domain.Entity, error) {
	if !repository.available() {
		return domain.Entity{}, sharedrepository.ErrUnavailable
	}
	if err := entity.Validate(); err != nil {
		return domain.Entity{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored domain.Entity
	err := repository.runtime.SQL.QueryRowContext(ctx, `
INSERT INTO entities (entity_key, entity_type, canonical_name, description, manual_locked)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (entity_key) DO UPDATE SET canonical_name = EXCLUDED.canonical_name, description = EXCLUDED.description, updated_at = now()
RETURNING id, version, entity_key, entity_type, canonical_name, description, manual_locked`, entity.Key, entity.Type, entity.Name, entity.Description, entity.ManualLocked).
		Scan(&stored.ID, &stored.Version, &stored.Key, &stored.Type, &stored.Name, &stored.Description, &stored.ManualLocked)
	if err != nil {
		return domain.Entity{}, sharedrepository.MapError(err)
	}
	return stored, nil
}

func (repository *Repository) SaveClaim(ctx context.Context, claim domain.Claim) (domain.Claim, error) {
	if !repository.available() {
		return domain.Claim{}, sharedrepository.ErrUnavailable
	}
	if err := claim.Validate(); err != nil {
		return domain.Claim{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored domain.Claim
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var claimID int64
		if err := transaction.SQL.QueryRowContext(ctx, `
SELECT id FROM event_claims WHERE event_id = $1 AND claim_hash = $2 FOR UPDATE`, claim.EventID, claim.ClaimHash).Scan(&claimID); err != nil && err != sql.ErrNoRows {
			return sharedrepository.MapError(err)
		}
		if claimID == 0 {
			if err := transaction.SQL.QueryRowContext(ctx, `INSERT INTO event_claims (event_id, normalized_claim, claim_hash, status, confidence, first_seen_at, last_seen_at, manual_locked) VALUES ($1,$2,$3,$4,$5,now(),now(),$6) RETURNING id`, claim.EventID, claim.NormalizedClaim, claim.ClaimHash, claim.Status, claim.Confidence, claim.ManualLocked).Scan(&claimID); err != nil {
				return sharedrepository.MapError(err)
			}
		} else if !claim.ManualLocked {
			if _, err := transaction.SQL.ExecContext(ctx, `UPDATE event_claims SET status = $1, confidence = $2, last_seen_at = now(), version = version + 1, updated_at = now() WHERE id = $3`, claim.Status, claim.Confidence, claimID); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		for _, evidence := range claim.Evidence {
			var active bool
			if err := transaction.SQL.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM contents c JOIN event_contents ec ON ec.content_id = c.id WHERE ec.event_id = $1 AND ec.content_id = $2 AND ec.evidence_role <> 'duplicate' AND c.content_status = 'active')`, claim.EventID, evidence.ContentID).Scan(&active); err != nil {
				return sharedrepository.MapError(err)
			}
			if !active {
				return fmt.Errorf("claim evidence content is not active in event")
			}
			if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO claim_evidences (claim_id, content_id, stance, evidence_locator, short_excerpt, confidence) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (claim_id, content_id, stance) DO UPDATE SET evidence_locator = EXCLUDED.evidence_locator, short_excerpt = EXCLUDED.short_excerpt, confidence = EXCLUDED.confidence, updated_at = now()`, claimID, evidence.ContentID, evidence.Stance, evidence.Locator, evidence.Excerpt, evidence.Confidence); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		stored = claim
		stored.ID, stored.Version = claimID, 1
		return nil
	})
	if err != nil {
		return domain.Claim{}, err
	}
	return stored, nil
}
