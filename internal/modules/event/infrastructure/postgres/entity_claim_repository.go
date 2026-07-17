package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		err := transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, entity_key, entity_type, canonical_name, description, manual_locked
FROM entities WHERE entity_key = $1 AND deleted_at IS NULL FOR UPDATE`, entity.Key).
			Scan(&stored.ID, &stored.Version, &stored.Key, &stored.Type, &stored.Name, &stored.Description, &stored.ManualLocked)
		if errors.Is(err, sql.ErrNoRows) {
			return transaction.SQL.QueryRowContext(ctx, `
INSERT INTO entities (entity_key, entity_type, canonical_name, description, manual_locked)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, version, entity_key, entity_type, canonical_name, description, manual_locked`, entity.Key, entity.Type, entity.Name, entity.Description, entity.ManualLocked).
				Scan(&stored.ID, &stored.Version, &stored.Key, &stored.Type, &stored.Name, &stored.Description, &stored.ManualLocked)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if stored.Type != entity.Type {
			return fmt.Errorf("%w: entity type is immutable", sharedrepository.ErrConflict)
		}
		if stored.ManualLocked {
			if stored.Name == entity.Name && stored.Description == entity.Description {
				return nil
			}
			return fmt.Errorf("%w: entity is manually locked", sharedrepository.ErrConflict)
		}
		return transaction.SQL.QueryRowContext(ctx, `
UPDATE entities SET canonical_name = $1, description = $2, manual_locked = $3, version = version + 1, updated_at = now()
WHERE id = $4
RETURNING id, version, entity_key, entity_type, canonical_name, description, manual_locked`, entity.Name, entity.Description, entity.ManualLocked, stored.ID).
			Scan(&stored.ID, &stored.Version, &stored.Key, &stored.Type, &stored.Name, &stored.Description, &stored.ManualLocked)
	})
	if err != nil {
		return domain.Entity{}, sharedrepository.MapError(err)
	}
	return stored, nil
}

func (repository *Repository) SaveEntityAlias(ctx context.Context, alias domain.EntityAlias) (domain.EntityAlias, error) {
	if !repository.available() {
		return domain.EntityAlias{}, sharedrepository.ErrUnavailable
	}
	if err := alias.Validate(); err != nil {
		return domain.EntityAlias{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored domain.EntityAlias
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		locked, err := entityLocked(ctx, transaction.SQL, alias.EntityID)
		if err != nil {
			return err
		}
		if locked && alias.Origin != domain.FactOriginUser {
			return fmt.Errorf("%w: entity is manually locked", sharedrepository.ErrConflict)
		}
		err = transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, entity_id, alias, normalized_alias, language, origin, confirmed
FROM entity_aliases
WHERE entity_id = $1 AND normalized_alias = $2 AND language = $3 FOR UPDATE`, alias.EntityID, alias.NormalizedAlias, alias.Language).
			Scan(&stored.ID, &stored.Version, &stored.EntityID, &stored.Alias, &stored.NormalizedAlias, &stored.Language, &stored.Origin, &stored.Confirmed)
		if errors.Is(err, sql.ErrNoRows) {
			return transaction.SQL.QueryRowContext(ctx, `
INSERT INTO entity_aliases (entity_id, alias, normalized_alias, language, origin, confirmed)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, version, entity_id, alias, normalized_alias, language, origin, confirmed`, alias.EntityID, alias.Alias, alias.NormalizedAlias, alias.Language, alias.Origin, alias.Confirmed).
				Scan(&stored.ID, &stored.Version, &stored.EntityID, &stored.Alias, &stored.NormalizedAlias, &stored.Language, &stored.Origin, &stored.Confirmed)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if stored.Confirmed && !alias.Confirmed {
			return fmt.Errorf("%w: entity alias is confirmed", sharedrepository.ErrConflict)
		}
		return transaction.SQL.QueryRowContext(ctx, `
UPDATE entity_aliases
SET alias = $1, origin = $2, confirmed = $3, version = version + 1, updated_at = now()
WHERE id = $4
RETURNING id, version, entity_id, alias, normalized_alias, language, origin, confirmed`, alias.Alias, alias.Origin, alias.Confirmed, stored.ID).
			Scan(&stored.ID, &stored.Version, &stored.EntityID, &stored.Alias, &stored.NormalizedAlias, &stored.Language, &stored.Origin, &stored.Confirmed)
	})
	if err != nil {
		return domain.EntityAlias{}, sharedrepository.MapError(err)
	}
	return stored, nil
}

func (repository *Repository) SaveEventEntity(ctx context.Context, eventEntity domain.EventEntity) (domain.EventEntity, error) {
	if !repository.available() {
		return domain.EventEntity{}, sharedrepository.ErrUnavailable
	}
	if err := eventEntity.Validate(); err != nil {
		return domain.EventEntity{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored domain.EventEntity
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		locked, err := eventOrEntityLocked(ctx, transaction.SQL, eventEntity.EventID, eventEntity.EntityID)
		if err != nil {
			return err
		}
		if locked && eventEntity.Origin != domain.FactOriginUser {
			return fmt.Errorf("%w: event or entity is manually locked", sharedrepository.ErrConflict)
		}
		err = transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_id, entity_id, role, confidence, origin, confirmed
FROM event_entities
WHERE event_id = $1 AND entity_id = $2 AND role = $3 FOR UPDATE`, eventEntity.EventID, eventEntity.EntityID, eventEntity.Role).
			Scan(&stored.ID, &stored.Version, &stored.EventID, &stored.EntityID, &stored.Role, &stored.Confidence, &stored.Origin, &stored.Confirmed)
		if errors.Is(err, sql.ErrNoRows) {
			return transaction.SQL.QueryRowContext(ctx, `
INSERT INTO event_entities (event_id, entity_id, role, confidence, origin, confirmed)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, version, event_id, entity_id, role, confidence, origin, confirmed`, eventEntity.EventID, eventEntity.EntityID, eventEntity.Role, eventEntity.Confidence, eventEntity.Origin, eventEntity.Confirmed).
				Scan(&stored.ID, &stored.Version, &stored.EventID, &stored.EntityID, &stored.Role, &stored.Confidence, &stored.Origin, &stored.Confirmed)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if stored.Confirmed && !eventEntity.Confirmed {
			return fmt.Errorf("%w: event entity is confirmed", sharedrepository.ErrConflict)
		}
		return transaction.SQL.QueryRowContext(ctx, `
UPDATE event_entities
SET confidence = $1, origin = $2, confirmed = $3, version = version + 1, updated_at = now()
WHERE id = $4
RETURNING id, version, event_id, entity_id, role, confidence, origin, confirmed`, eventEntity.Confidence, eventEntity.Origin, eventEntity.Confirmed, stored.ID).
			Scan(&stored.ID, &stored.Version, &stored.EventID, &stored.EntityID, &stored.Role, &stored.Confidence, &stored.Origin, &stored.Confirmed)
	})
	if err != nil {
		return domain.EventEntity{}, sharedrepository.MapError(err)
	}
	return stored, nil
}

func (repository *Repository) SaveEntityRelation(ctx context.Context, relation domain.EntityRelation) (domain.EntityRelation, error) {
	if !repository.available() {
		return domain.EntityRelation{}, sharedrepository.ErrUnavailable
	}
	if err := relation.Validate(); err != nil {
		return domain.EntityRelation{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored domain.EntityRelation
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		locked, err := entitiesLocked(ctx, transaction.SQL, relation.FromEntityID, relation.ToEntityID)
		if err != nil {
			return err
		}
		if locked && relation.Origin != domain.FactOriginUser {
			return fmt.Errorf("%w: entity is manually locked", sharedrepository.ErrConflict)
		}
		err = transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, from_entity_id, to_entity_id, relation_type, confidence, valid_from, valid_to, origin, confirmed
FROM entity_relations
WHERE from_entity_id = $1 AND to_entity_id = $2 AND relation_type = $3 AND valid_from IS NOT DISTINCT FROM $4 FOR UPDATE`, relation.FromEntityID, relation.ToEntityID, relation.Type, nullableTime(relation.ValidFrom)).
			Scan(&stored.ID, &stored.Version, &stored.FromEntityID, &stored.ToEntityID, &stored.Type, &stored.Confidence, &stored.ValidFrom, &stored.ValidTo, &stored.Origin, &stored.Confirmed)
		if errors.Is(err, sql.ErrNoRows) {
			return transaction.SQL.QueryRowContext(ctx, `
INSERT INTO entity_relations (from_entity_id, to_entity_id, relation_type, confidence, valid_from, valid_to, origin, confirmed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, version, from_entity_id, to_entity_id, relation_type, confidence, valid_from, valid_to, origin, confirmed`, relation.FromEntityID, relation.ToEntityID, relation.Type, relation.Confidence, nullableTime(relation.ValidFrom), nullableTime(relation.ValidTo), relation.Origin, relation.Confirmed).
				Scan(&stored.ID, &stored.Version, &stored.FromEntityID, &stored.ToEntityID, &stored.Type, &stored.Confidence, &stored.ValidFrom, &stored.ValidTo, &stored.Origin, &stored.Confirmed)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if stored.Confirmed && !relation.Confirmed {
			return fmt.Errorf("%w: entity relation is confirmed", sharedrepository.ErrConflict)
		}
		return transaction.SQL.QueryRowContext(ctx, `
UPDATE entity_relations
SET confidence = $1, valid_to = $2, origin = $3, confirmed = $4, version = version + 1, updated_at = now()
WHERE id = $5
RETURNING id, version, from_entity_id, to_entity_id, relation_type, confidence, valid_from, valid_to, origin, confirmed`, relation.Confidence, nullableTime(relation.ValidTo), relation.Origin, relation.Confirmed, stored.ID).
			Scan(&stored.ID, &stored.Version, &stored.FromEntityID, &stored.ToEntityID, &stored.Type, &stored.Confidence, &stored.ValidFrom, &stored.ValidTo, &stored.Origin, &stored.Confirmed)
	})
	if err != nil {
		return domain.EntityRelation{}, sharedrepository.MapError(err)
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
		for _, evidence := range claim.Evidence {
			var active bool
			if err := transaction.SQL.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM contents content
    JOIN event_contents membership ON membership.content_id = content.id
    WHERE membership.event_id = $1 AND membership.content_id = $2
      AND membership.evidence_role <> 'duplicate'
      AND content.content_status = 'active' AND content.deleted_at IS NULL
)`, claim.EventID, evidence.ContentID).Scan(&active); err != nil {
				return sharedrepository.MapError(err)
			}
			if !active {
				return fmt.Errorf("%w: claim evidence content is not active in event", sharedrepository.ErrConflict)
			}
		}

		err := transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, manual_locked FROM event_claims
WHERE event_id = $1 AND claim_hash = $2 FOR UPDATE`, claim.EventID, claim.ClaimHash).
			Scan(&stored.ID, &stored.Version, &stored.ManualLocked)
		if errors.Is(err, sql.ErrNoRows) {
			err = transaction.SQL.QueryRowContext(ctx, `
INSERT INTO event_claims (event_id, normalized_claim, claim_hash, status, confidence, first_seen_at, last_seen_at, manual_locked)
VALUES ($1,$2,$3,$4,$5,now(),now(),$6)
RETURNING id, version, manual_locked`, claim.EventID, claim.NormalizedClaim, claim.ClaimHash, claim.Status, claim.Confidence, claim.ManualLocked).
				Scan(&stored.ID, &stored.Version, &stored.ManualLocked)
		} else if err == nil {
			if stored.ManualLocked {
				return fmt.Errorf("%w: claim is manually locked", sharedrepository.ErrConflict)
			}
			err = transaction.SQL.QueryRowContext(ctx, `
UPDATE event_claims
SET normalized_claim = $1, status = $2, confidence = $3, manual_locked = $4, last_seen_at = now(), version = version + 1, updated_at = now()
WHERE id = $5
RETURNING id, version, manual_locked`, claim.NormalizedClaim, claim.Status, claim.Confidence, claim.ManualLocked, stored.ID).
				Scan(&stored.ID, &stored.Version, &stored.ManualLocked)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		for _, evidence := range claim.Evidence {
			if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO claim_evidences (claim_id, content_id, stance, evidence_locator, short_excerpt, confidence)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (claim_id, content_id, stance) DO UPDATE
SET evidence_locator = EXCLUDED.evidence_locator, short_excerpt = EXCLUDED.short_excerpt, confidence = EXCLUDED.confidence, version = claim_evidences.version + 1, updated_at = now()`, stored.ID, evidence.ContentID, evidence.Stance, evidence.Locator, evidence.Excerpt, evidence.Confidence); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		stored.EventID, stored.NormalizedClaim, stored.ClaimHash, stored.Status, stored.Confidence, stored.Evidence = claim.EventID, claim.NormalizedClaim, claim.ClaimHash, claim.Status, claim.Confidence, append([]domain.ClaimEvidence(nil), claim.Evidence...)
		return nil
	})
	if err != nil {
		return domain.Claim{}, sharedrepository.MapError(err)
	}
	return stored, nil
}

func entityLocked(ctx context.Context, query *sql.Tx, entityID int64) (bool, error) {
	var locked bool
	err := query.QueryRowContext(ctx, `SELECT manual_locked FROM entities WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, entityID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%w: entity", sharedrepository.ErrNotFound)
	}
	return locked, sharedrepository.MapError(err)
}

func eventOrEntityLocked(ctx context.Context, query *sql.Tx, eventID, entityID int64) (bool, error) {
	var eventLocked, entityLocked bool
	err := query.QueryRowContext(ctx, `
SELECT event.manual_locked, entity.manual_locked
FROM events event JOIN entities entity ON entity.id = $2
WHERE event.id = $1 AND event.deleted_at IS NULL AND entity.deleted_at IS NULL
FOR UPDATE OF event, entity`, eventID, entityID).Scan(&eventLocked, &entityLocked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%w: event or entity", sharedrepository.ErrNotFound)
	}
	return eventLocked || entityLocked, sharedrepository.MapError(err)
}

func entitiesLocked(ctx context.Context, query *sql.Tx, firstID, secondID int64) (bool, error) {
	rows, err := query.QueryContext(ctx, `SELECT id, manual_locked FROM entities WHERE id = ANY($1) AND deleted_at IS NULL ORDER BY id FOR UPDATE`, []int64{firstID, secondID})
	if err != nil {
		return false, sharedrepository.MapError(err)
	}
	defer rows.Close()
	count, locked := 0, false
	for rows.Next() {
		var id int64
		var value bool
		if err := rows.Scan(&id, &value); err != nil {
			return false, sharedrepository.MapError(err)
		}
		count++
		locked = locked || value
	}
	if err := rows.Err(); err != nil {
		return false, sharedrepository.MapError(err)
	}
	if count != 2 {
		return false, fmt.Errorf("%w: entity relation endpoint", sharedrepository.ErrNotFound)
	}
	return locked, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
