//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRepositoryPersistsExtractedFactsAtomically(t *testing.T) {
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
	facts := extractedFactsFixture(fixture.sourceID, fixture.sourceContentID, strings.Repeat("c", 64), "acme", "Acme")
	stored, err := repository.PersistExtractedFacts(ctx, facts)
	if err != nil || len(stored.Entities) != 1 || len(stored.Claims) != 1 {
		t.Fatalf("PersistExtractedFacts() = %#v / %v", stored, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE entities SET manual_locked = true WHERE id = $1`, stored.Entities[0].ID); err != nil {
		t.Fatal(err)
	}
	rollback := application.ExtractedEventFacts{EventID: fixture.sourceID, Entities: []application.ExtractedEventEntity{
		{Entity: domain.Entity{ID: 1, Version: 1, Key: "newco", Name: "Newco", Type: domain.EntityOrganization}, Alias: "Newco", NormalizedAlias: "newco", Language: "en", Role: "mentioned", Confidence: 50},
		{Entity: domain.Entity{ID: 1, Version: 1, Key: "acme", Name: "Changed Acme", Type: domain.EntityOrganization}, Alias: "Changed Acme", NormalizedAlias: "changed acme", Language: "en", Role: "mentioned", Confidence: 50},
	}, Claims: []domain.Claim{claimFixture(fixture.sourceID, fixture.sourceContentID, strings.Repeat("d", 64))}}
	if _, err := repository.PersistExtractedFacts(ctx, rollback); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("PersistExtractedFacts(locked conflict) error = %v, want conflict", err)
	}
	var entities, aliases, eventEntities, claims, evidences int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM entities`).Scan(&entities); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM entity_aliases`).Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_entities WHERE event_id = $1`, fixture.sourceID).Scan(&eventEntities); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_claims WHERE event_id = $1`, fixture.sourceID).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM claim_evidences`).Scan(&evidences); err != nil {
		t.Fatal(err)
	}
	if entities != 1 || aliases != 1 || eventEntities != 1 || claims != 1 || evidences != 1 {
		t.Fatalf("facts after rollback = entities:%d aliases:%d event_entities:%d claims:%d evidences:%d, want 1 each", entities, aliases, eventEntities, claims, evidences)
	}
}

func TestRepositoryReadsSafeEventIntelligenceFacts(t *testing.T) {
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
	if _, err := repository.PersistExtractedFacts(ctx, extractedFactsFixture(fixture.sourceID, fixture.sourceContentID, strings.Repeat("e", 64), "acme", "Acme")); err != nil {
		t.Fatal(err)
	}
	result, err := repository.ReadEventIntelligence(ctx, fixture.sourceID)
	if err != nil {
		t.Fatalf("ReadEventIntelligence() error = %v", err)
	}
	if result.EventID != fixture.sourceID || len(result.Entities) != 1 || result.Entities[0].Entity.Key != "acme" || len(result.Claims) != 1 || len(result.Claims[0].Evidence) != 1 || result.Claims[0].Evidence[0].ContentID != fixture.sourceContentID {
		t.Fatalf("ReadEventIntelligence() = %#v", result)
	}
}

func extractedFactsFixture(eventID, contentID int64, hash, key, name string) application.ExtractedEventFacts {
	return application.ExtractedEventFacts{EventID: eventID, Entities: []application.ExtractedEventEntity{{Entity: domain.Entity{ID: 1, Version: 1, Key: key, Name: name, Type: domain.EntityOrganization}, Alias: name, NormalizedAlias: strings.ToLower(name), Language: "en", Role: "mentioned", Confidence: 50}}, Claims: []domain.Claim{claimFixture(eventID, contentID, hash)}}
}
