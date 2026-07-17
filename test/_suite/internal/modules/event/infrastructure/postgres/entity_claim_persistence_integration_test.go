//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestEntityFactPersistenceProtectsConfirmedAndLockedValues(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	fixture := seedEventFixture(t, runtime)
	first, err := repository.SaveEntity(ctx, entityFixture("acme", "Acme"))
	if err != nil {
		t.Fatalf("SaveEntity(first) error = %v", err)
	}
	second, err := repository.SaveEntity(ctx, entityFixture("globex", "Globex"))
	if err != nil {
		t.Fatalf("SaveEntity(second) error = %v", err)
	}

	alias := domain.EntityAlias{EntityID: first.ID, Alias: "Acme", NormalizedAlias: "acme", Language: "en", Origin: domain.FactOriginModel}
	if _, err := repository.SaveEntityAlias(ctx, alias); err != nil {
		t.Fatalf("SaveEntityAlias(model) error = %v", err)
	}
	alias.Alias, alias.Origin, alias.Confirmed = "Acme Corporation", domain.FactOriginUser, true
	confirmedAlias, err := repository.SaveEntityAlias(ctx, alias)
	if err != nil || !confirmedAlias.Confirmed || confirmedAlias.Version != 2 {
		t.Fatalf("SaveEntityAlias(confirm) = %#v / %v", confirmedAlias, err)
	}
	alias.Alias, alias.Origin, alias.Confirmed = "Acme AI", domain.FactOriginModel, false
	if _, err := repository.SaveEntityAlias(ctx, alias); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("SaveEntityAlias(overwrite confirmed) error = %v, want conflict", err)
	}

	eventEntity := domain.EventEntity{EventID: fixture.sourceID, EntityID: first.ID, Role: "mentioned", Confidence: 60, Origin: domain.FactOriginModel}
	if _, err := repository.SaveEventEntity(ctx, eventEntity); err != nil {
		t.Fatalf("SaveEventEntity(model) error = %v", err)
	}
	eventEntity.Confidence, eventEntity.Origin, eventEntity.Confirmed = 90, domain.FactOriginUser, true
	if _, err := repository.SaveEventEntity(ctx, eventEntity); err != nil {
		t.Fatalf("SaveEventEntity(confirm) error = %v", err)
	}
	eventEntity.Confidence, eventEntity.Origin, eventEntity.Confirmed = 40, domain.FactOriginModel, false
	if _, err := repository.SaveEventEntity(ctx, eventEntity); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("SaveEventEntity(overwrite confirmed) error = %v, want conflict", err)
	}

	relation := domain.EntityRelation{FromEntityID: first.ID, ToEntityID: second.ID, Type: domain.EntityRelationRelatedTo, Confidence: 60, Origin: domain.FactOriginModel}
	if _, err := repository.SaveEntityRelation(ctx, relation); err != nil {
		t.Fatalf("SaveEntityRelation(model) error = %v", err)
	}
	relation.Confidence, relation.Origin, relation.Confirmed = 90, domain.FactOriginUser, true
	if _, err := repository.SaveEntityRelation(ctx, relation); err != nil {
		t.Fatalf("SaveEntityRelation(confirm) error = %v", err)
	}
	relation.Confidence, relation.Origin, relation.Confirmed = 40, domain.FactOriginModel, false
	if _, err := repository.SaveEntityRelation(ctx, relation); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("SaveEntityRelation(overwrite confirmed) error = %v, want conflict", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO entity_relations (from_entity_id, to_entity_id, relation_type, confidence, origin) VALUES ($1,$2,'invented',50,'model')`, first.ID, second.ID); err == nil {
		t.Fatal("database accepted an uncontrolled entity relation type")
	}
	if _, err := runtime.SQL.Exec(`UPDATE entities SET manual_locked = true WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	alias.Origin = domain.FactOriginModel
	if _, err := repository.SaveEntityAlias(ctx, alias); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("SaveEntityAlias(locked entity) error = %v, want conflict", err)
	}
}

func TestClaimPersistenceRejectsInactiveEvidenceAndManualOverwrite(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	fixture := seedEventFixture(t, runtime)
	claim := claimFixture(fixture.sourceID, fixture.sourceContentID, strings.Repeat("a", 64))
	stored, err := repository.SaveClaim(ctx, claim)
	if err != nil || stored.ID <= 0 || stored.Version != 1 {
		t.Fatalf("SaveClaim(first) = %#v / %v", stored, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE event_claims SET manual_locked = true WHERE id = $1`, stored.ID); err != nil {
		t.Fatal(err)
	}
	claim.Confidence = 10
	if _, err := repository.SaveClaim(ctx, claim); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("SaveClaim(overwrite locked) error = %v, want conflict", err)
	}
	var confidence float64
	if err := runtime.SQL.QueryRow(`SELECT confidence FROM event_claims WHERE id = $1`, stored.ID).Scan(&confidence); err != nil {
		t.Fatal(err)
	}
	if confidence != 80 {
		t.Fatalf("locked claim confidence = %v, want 80", confidence)
	}

	if _, err := runtime.SQL.Exec(`UPDATE contents SET content_status = 'deleted' WHERE id = $1`, fixture.sourceContentID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SaveClaim(ctx, claimFixture(fixture.sourceID, fixture.sourceContentID, strings.Repeat("b", 64))); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("SaveClaim(inactive evidence) error = %v, want conflict", err)
	}
	var claims, evidences int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_claims WHERE event_id = $1`, fixture.sourceID).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM claim_evidences WHERE claim_id = $1`, stored.ID).Scan(&evidences); err != nil {
		t.Fatal(err)
	}
	if claims != 1 || evidences != 1 {
		t.Fatalf("claims/evidences after rollback = %d/%d, want 1/1", claims, evidences)
	}
}

func entityFixture(key, name string) domain.Entity {
	return domain.Entity{ID: 1, Version: 1, Key: key, Name: name, Type: domain.EntityOrganization}
}

func claimFixture(eventID, contentID int64, hash string) domain.Claim {
	return domain.Claim{ID: 1, Version: 1, EventID: eventID, NormalizedClaim: "the event happened", ClaimHash: hash, Status: domain.ClaimSingleSource, Confidence: 80, Evidence: []domain.ClaimEvidence{{EvidenceRef: domain.EvidenceRef{ContentID: contentID, Locator: "title", Excerpt: "Event content"}, Stance: "supports", Confidence: 90}}}
}
