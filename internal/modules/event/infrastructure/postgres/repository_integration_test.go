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
	if _, err := runtime.SQL.Exec(`UPDATE events SET representative_content_id = $1 WHERE id = $2`, fixture.sourceContentID, fixture.sourceID); err != nil {
		t.Fatalf("set source representative: %v", err)
	}
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
	} else if got.LifecycleStatus != domain.LifecycleMerged || got.MergedIntoID == nil || *got.MergedIntoID != fixture.targetID || got.RepresentativeContentID != nil {
		t.Fatalf("source after merge = %#v", got)
	}
	var movedContentID int64
	if err := runtime.SQL.QueryRow(`SELECT (metadata->>'content_id')::bigint FROM event_governance_audits WHERE event_id = $1 AND action = 'member_move'`, fixture.targetID).Scan(&movedContentID); err != nil {
		t.Fatalf("read member move audit: %v", err)
	}
	if movedContentID != fixture.sourceContentID {
		t.Fatalf("member move audit content = %d, want %d", movedContentID, fixture.sourceContentID)
	}
}

func TestGovernanceRepositoryMergeRecomputesTargetProjection(t *testing.T) {
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
	targetContentID := seedUnassignedEventContent(t, runtime)
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, is_representative, origin) VALUES ($1,$2,80,'primary',true,'rule')`, fixture.targetID, targetContentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE events SET representative_content_id = $1 WHERE id = $2`, targetContentID, fixture.targetID); err != nil {
		t.Fatal(err)
	}

	merged, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: fixture.targetID, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "merge"})
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	if merged.LifecycleStatus != domain.LifecycleActive || merged.RepresentativeContentID == nil || *merged.RepresentativeContentID != fixture.sourceContentID {
		t.Fatalf("merged target = %#v, want active event represented by higher-score source content", merged)
	}
	var recomputes int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_governance_audits WHERE event_id = $1 AND action = 'evidence_recompute'`, fixture.targetID).Scan(&recomputes); err != nil {
		t.Fatal(err)
	}
	if recomputes != 1 {
		t.Fatalf("target evidence recompute audits = %d, want 1", recomputes)
	}
}

func TestGovernanceRepositoryMergesThroughCanonicalTargetUsingCanonicalVersion(t *testing.T) {
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
	if _, err := runtime.SQL.Exec(`UPDATE events SET version = 2 WHERE id = $1`, fixture.targetID); err != nil {
		t.Fatalf("bump canonical target version: %v", err)
	}
	var historicalTargetID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at, merged_into_id) SELECT 'evt-history-' || md5(random()::text), '历史事件', '', 'merged', first_seen_at, last_seen_at, $1 FROM events WHERE id = $1 RETURNING id`, fixture.targetID).Scan(&historicalTargetID); err != nil {
		t.Fatalf("insert merged target: %v", err)
	}
	merged, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: historicalTargetID, SourceExpectedVersion: 1, TargetExpectedVersion: 2, ReasonCode: "canonical_merge"})
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	if merged.ID != fixture.targetID || merged.Version != 3 {
		t.Fatalf("Merge() = %#v, want canonical target version 3", merged)
	}
	if source, err := repository.Get(ctx, fixture.sourceID); err != nil {
		t.Fatalf("Get(source) error = %v", err)
	} else if source.LifecycleStatus != domain.LifecycleMerged || source.MergedIntoID == nil || *source.MergedIntoID != fixture.targetID {
		t.Fatalf("source after canonical merge = %#v", source)
	}
}

func TestGovernanceRepositoryRejectsStaleCanonicalTargetVersion(t *testing.T) {
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
	if _, err := runtime.SQL.Exec(`UPDATE events SET version = 2 WHERE id = $1`, fixture.targetID); err != nil {
		t.Fatalf("set client canonical version: %v", err)
	}
	var historicalTargetID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at, merged_into_id) SELECT 'evt-history-' || md5(random()::text), '历史事件', '', 'merged', first_seen_at, last_seen_at, $1 FROM events WHERE id = $1 RETURNING id`, fixture.targetID).Scan(&historicalTargetID); err != nil {
		t.Fatalf("insert merged target: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE events SET version = 2 WHERE id = $1`, historicalTargetID); err != nil {
		t.Fatalf("set historical target version: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE events SET version = 3 WHERE id = $1`, fixture.targetID); err != nil {
		t.Fatalf("make canonical target stale: %v", err)
	}
	if _, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: historicalTargetID, SourceExpectedVersion: 1, TargetExpectedVersion: 2, ReasonCode: "canonical_merge"}); err == nil {
		t.Fatal("Merge() accepted a stale canonical target version")
	}
	if source, err := repository.Get(ctx, fixture.sourceID); err != nil {
		t.Fatal(err)
	} else if source.LifecycleStatus != domain.LifecycleDetected || source.MergedIntoID != nil {
		t.Fatalf("source changed after stale canonical conflict: %#v", source)
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
	if created.EventFingerprint != "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" || created.FingerprintVersion != "content_dedupe_v1" {
		t.Fatalf("Split() fingerprint = %q/%q, want moved content fingerprint/content_dedupe_v1", created.EventFingerprint, created.FingerprintVersion)
	}
	if got, err := repository.Get(ctx, fixture.sourceID); err != nil {
		t.Fatalf("Get(source) error = %v", err)
	} else if got.Version != 2 {
		t.Fatalf("source version after split = %d, want 2", got.Version)
	}
	var movedContentID int64
	if err := runtime.SQL.QueryRow(`SELECT (metadata->>'content_id')::bigint FROM event_governance_audits WHERE event_id = $1 AND action = 'member_move'`, created.ID).Scan(&movedContentID); err != nil {
		t.Fatalf("read split member move audit: %v", err)
	}
	if movedContentID != fixture.sourceContentID {
		t.Fatalf("split member move audit content = %d, want %d", movedContentID, fixture.sourceContentID)
	}
}

func TestGovernanceRepositoryAuditsMonitorDeduplication(t *testing.T) {
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
	var monitorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ('governance-monitor-' || md5(random()::text)) RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	now := time.Now().UTC()
	for _, eventID := range []int64{fixture.sourceID, fixture.targetID} {
		if _, err := runtime.SQL.Exec(`INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at) VALUES ($1,$2,80,80,$3,$3)`, monitorID, eventID, now); err != nil {
			t.Fatalf("insert monitor event: %v", err)
		}
	}
	if _, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: fixture.targetID, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "merge"}); err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	var auditedMonitorID int64
	if err := runtime.SQL.QueryRow(`SELECT (metadata->>'monitor_id')::bigint FROM event_governance_audits WHERE event_id = $1 AND action = 'deduplicated' AND metadata->>'relation' = 'monitor_event'`, fixture.targetID).Scan(&auditedMonitorID); err != nil {
		t.Fatalf("read monitor deduplication audit: %v", err)
	}
	if auditedMonitorID != monitorID {
		t.Fatalf("deduplicated monitor = %d, want %d", auditedMonitorID, monitorID)
	}
}

func TestGovernanceRepositorySplitRecomputesBothEventProjections(t *testing.T) {
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
	remainingContentID := seedUnassignedEventContent(t, runtime)
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, origin) VALUES ($1,$2,80,'supporting','rule')`, fixture.sourceID, remainingContentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE event_contents SET is_representative = (content_id = $1) WHERE event_id = $2`, fixture.sourceContentID, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE events SET lifecycle_status = 'active', representative_content_id = $1 WHERE id = $2`, fixture.sourceContentID, fixture.sourceID); err != nil {
		t.Fatal(err)
	}

	created, err := repository.Split(ctx, application.SplitCommand{SourceEventID: fixture.sourceID, SourceExpectedVersion: 1, Members: []application.SplitMember{{ContentID: fixture.sourceContentID, ExpectedVersion: 1}}, ReasonCode: "separate"})
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	if created.LifecycleStatus != domain.LifecycleDetected || created.RepresentativeContentID == nil || *created.RepresentativeContentID != fixture.sourceContentID {
		t.Fatalf("created event = %#v, want detected event represented by moved content", created)
	}
	source, err := repository.Get(ctx, fixture.sourceID)
	if err != nil {
		t.Fatalf("Get(source) error = %v", err)
	}
	if source.LifecycleStatus != domain.LifecycleCooling || source.RepresentativeContentID == nil || *source.RepresentativeContentID != remainingContentID {
		t.Fatalf("source event = %#v, want cooling event represented by remaining content", source)
	}
	var recomputes int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_governance_audits WHERE action = 'evidence_recompute' AND event_id IN ($1,$2)`, fixture.sourceID, created.ID).Scan(&recomputes); err != nil {
		t.Fatal(err)
	}
	if recomputes != 2 {
		t.Fatalf("evidence recompute audits = %d, want 2", recomputes)
	}
}

func TestGovernanceRepositorySplitRejectsStaleMemberWithoutPartialMove(t *testing.T) {
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
	secondContentID := seedUnassignedEventContent(t, runtime)
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, origin) VALUES ($1,$2,80,'supporting','rule')`, fixture.sourceID, secondContentID); err != nil {
		t.Fatalf("insert second member: %v", err)
	}
	_, err = repository.Split(ctx, application.SplitCommand{SourceEventID: fixture.sourceID, SourceExpectedVersion: 1, Members: []application.SplitMember{{ContentID: secondContentID, ExpectedVersion: 1}, {ContentID: fixture.sourceContentID, ExpectedVersion: 2}}, ReasonCode: "separate"})
	if err == nil {
		t.Fatal("Split() accepted stale member version")
	}
	var sourceMembers, createdEvents int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE event_id = $1`, fixture.sourceID).Scan(&sourceMembers); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM events WHERE event_key LIKE 'evt_%'`).Scan(&createdEvents); err != nil {
		t.Fatal(err)
	}
	if sourceMembers != 2 || createdEvents != 2 {
		t.Fatalf("source members/events = %d/%d, want 2/2 after rollback", sourceMembers, createdEvents)
	}
}

func TestGovernanceRepositoryRollsBackMergeAndSplitOnMemberMoveFailure(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []struct {
		name string
		run  func(*Repository, struct{ sourceID, targetID, sourceContentID int64 }) error
	}{
		{
			name: "merge",
			run: func(repository *Repository, fixture struct{ sourceID, targetID, sourceContentID int64 }) error {
				_, err := repository.Merge(ctx, application.MergeCommand{SourceEventID: fixture.sourceID, TargetEventID: fixture.targetID, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: "fault_injection"})
				return err
			},
		},
		{
			name: "split",
			run: func(repository *Repository, fixture struct{ sourceID, targetID, sourceContentID int64 }) error {
				_, err := repository.Split(ctx, application.SplitCommand{SourceEventID: fixture.sourceID, SourceExpectedVersion: 1, Members: []application.SplitMember{{ContentID: fixture.sourceContentID, ExpectedVersion: 1}}, ReasonCode: "fault_injection"})
				return err
			},
		},
	} {
		t.Run(operation.name, func(t *testing.T) {
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
			if _, err := runtime.SQL.Exec(`
CREATE FUNCTION fail_event_member_move() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.event_id <> OLD.event_id THEN
    RAISE EXCEPTION 'injected event member move failure';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER fail_event_member_move BEFORE UPDATE OF event_id ON event_contents
FOR EACH ROW EXECUTE FUNCTION fail_event_member_move();`); err != nil {
				t.Fatalf("install failure trigger: %v", err)
			}
			if err := operation.run(repository, fixture); err == nil {
				t.Fatal("governance operation unexpectedly succeeded")
			}
			var sourceMembers, targetMembers, events, audits int
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE event_id = $1`, fixture.sourceID).Scan(&sourceMembers); err != nil {
				t.Fatal(err)
			}
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE event_id = $1`, fixture.targetID).Scan(&targetMembers); err != nil {
				t.Fatal(err)
			}
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM events`).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_governance_audits`).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if sourceMembers != 1 || targetMembers != 0 || events != 2 || audits != 0 {
				t.Fatalf("members/events/audits = %d/%d/%d/%d, want 1/0/2/0 after rollback", sourceMembers, targetMembers, events, audits)
			}
		})
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
