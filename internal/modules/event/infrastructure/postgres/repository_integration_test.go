//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestGovernanceRepositoryMergesAndLocksAtomically(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatalf("database.InitializeEmpty() error = %v", err)
	}
	repository := NewRepository(runtime)
	fixture := seedEventFixture(t, runtime)
	member, err := repository.SetMemberLock(ctx, application.MemberLockCommand{EventID: fixture.sourceID, ContentID: fixture.sourceContentID, ExpectedVersion: 1, Locked: true, ReasonCode: "review"})
	if err != nil {
		t.Fatalf("SetMemberLock() error = %v", err)
	}
	if !member.ManualLocked || member.Version != 2 {
		t.Fatalf("SetMemberLock() = %#v, want locked version 2", member)
	}
	if _, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: fixture.targetID, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "merge"}); err == nil {
		t.Fatal("Merge() accepted locked member")
	}
	if _, err := repository.SetMemberLock(ctx, application.MemberLockCommand{EventID: fixture.sourceID, ContentID: fixture.sourceContentID, ExpectedVersion: 2, Locked: false, ReasonCode: "reviewed"}); err != nil {
		t.Fatalf("SetMemberLock(unlock) error = %v", err)
	}
	merged, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: fixture.targetID, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "merge"})
	if err != nil || merged.ID != fixture.targetID || merged.Version != 2 {
		t.Fatalf("Merge() = %#v/%v, want target version 2", merged, err)
	}
	if got, err := repository.Get(ctx, fixture.sourceID); err != nil {
		t.Fatalf("Get(source) error = %v", err)
	} else if got.LifecycleStatus != domain.LifecycleMerged || got.MergedIntoID == nil || *got.MergedIntoID != fixture.targetID {
		t.Fatalf("source after merge = %#v", got)
	}
}

func TestGovernanceRepositorySplitsWithMemberVersion(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatalf("database.InitializeEmpty() error = %v", err)
	}
	repository := NewRepository(runtime)
	fixture := seedEventFixture(t, runtime)
	created, err := repository.Split(ctx, application.SplitCommand{SourceEventID: fixture.sourceID, SourceExpectedVersion: 1, Members: []application.SplitMember{{ContentID: fixture.sourceContentID, ExpectedVersion: 1}}, ReasonCode: "separate"})
	if err != nil || created.ID == 0 || created.LifecycleStatus != domain.LifecycleDetected {
		t.Fatalf("Split() = %#v/%v", created, err)
	}
	if got, err := repository.Get(ctx, fixture.sourceID); err != nil {
		t.Fatalf("Get(source) error = %v", err)
	} else if got.Version != 2 {
		t.Fatalf("source version after split = %d, want 2", got.Version)
	}
}

func seedEventFixture(t *testing.T, runtime *database.Runtime) struct{ sourceID, targetID, sourceContentID int64 } {
	t.Helper()
	var fixture struct{ sourceID, targetID, sourceContentID int64 }
	now := time.Now().UTC()
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'event-fixture-' || md5(random()::text), 'https://event.example') RETURNING id`).Scan(&fixture.sourceContentID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (source_connection_id, external_id, content_type, title, canonical_url, published_at, fetched_at, dedupe_key) VALUES ($1, 'event-content', 'article', 'Event content', 'https://event.example/content', $2, $2, repeat('e',64)) RETURNING id`, fixture.sourceContentID, now).Scan(&fixture.sourceContentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('evt-source-' || md5(random()::text), '源事件', '', 'detected', $1, $1) RETURNING id`, now).Scan(&fixture.sourceID); err != nil {
		t.Fatalf("insert source event: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('evt-target-' || md5(random()::text), '目标事件', '', 'active', $1, $1) RETURNING id`, now).Scan(&fixture.targetID); err != nil {
		t.Fatalf("insert target event: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, origin) VALUES ($1,$2,90,'primary','rule')`, fixture.sourceID, fixture.sourceContentID); err != nil {
		t.Fatalf("insert event member: %v", err)
	}
	return fixture
}
