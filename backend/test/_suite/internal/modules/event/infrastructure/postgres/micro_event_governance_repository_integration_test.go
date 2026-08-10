//go:build integration

package postgres

import (
	"context"
	"fmt"
	"testing"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestMicroEventGovernanceMovesSplitsMergesClosesReopensAndReplays(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var actorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
	VALUES ('event-governor@example.test','fixture','Event Governor','editor') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	microEventRepository, _ := NewMicroEventRepository(runtime)
	microEventService, _ := eventapplication.NewMicroEventService(microEventRepository)
	governanceRepository, _ := NewMicroEventGovernancePostgresRepository(runtime)
	service, _ := eventapplication.NewMicroEventGovernanceService(governanceRepository)

	firstFixture := seedMicroEventAssignmentFixture(t, runtime, "governance-first", "accepted")
	first, err := microEventService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: firstFixture.familyID, DocumentMatchDecisionID: firstFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	secondFixture := seedMicroEventAssignmentFixture(t, runtime, "governance-second", "accepted")
	setMicroEventFixtureAction(t, runtime, secondFixture.documentVersionID, "action:response")
	second, err := microEventService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: secondFixture.familyID, DocumentMatchDecisionID: secondFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil || second.Event.ID == first.Event.ID {
		t.Fatalf("second event = %#v / %v", second, err)
	}

	moved, err := service.Govern(ctx, governMicroEventFixture(actorID, "move_member", first.Event.ID,
		first.Event.Version, first.Decision.ID, firstFixture.familyID, 1, second.Event.ID, second.Event.Version, "move-a"))
	if err != nil || moved.SourceEvent.Status != "closed" || moved.TargetEvent == nil ||
		moved.Feedback.ResultMembershipDecisionID <= 0 || moved.Feedback.ResultMemberVersion != 2 {
		t.Fatalf("move = %#v / %v", moved, err)
	}
	replayed, err := service.Govern(ctx, governMicroEventFixture(actorID, "move_member", first.Event.ID,
		first.Event.Version, first.Decision.ID, firstFixture.familyID, 1, second.Event.ID, second.Event.Version, "move-a"))
	if err != nil || replayed.Feedback.ID != moved.Feedback.ID {
		t.Fatalf("move replay = %#v / %v", replayed, err)
	}

	reopened, err := service.Govern(ctx, governMicroEventFixture(actorID, "reopen_event", first.Event.ID,
		moved.SourceEvent.Version, 0, 0, 0, 0, 0, "reopen-a"))
	if err != nil || reopened.SourceEvent.Status != "active" {
		t.Fatalf("reopen = %#v / %v", reopened, err)
	}
	split, err := service.Govern(ctx, governMicroEventFixture(actorID, "split_event", second.Event.ID,
		moved.TargetEvent.Version, moved.Feedback.ResultMembershipDecisionID, firstFixture.familyID,
		moved.Feedback.ResultMemberVersion, 0, 0, "split-a"))
	if err != nil || split.TargetEvent == nil || split.TargetEvent.ID == second.Event.ID ||
		split.Feedback.ResultMembershipDecisionID <= 0 {
		t.Fatalf("split = %#v / %v", split, err)
	}
	merged, err := service.Govern(ctx, governMicroEventFixture(actorID, "merge_events", split.TargetEvent.ID,
		split.TargetEvent.Version, 0, 0, 0, reopened.SourceEvent.ID, reopened.SourceEvent.Version, "merge-a"))
	if err != nil || merged.SourceEvent.Status != "merged" || merged.TargetEvent == nil ||
		merged.TargetEvent.ID != reopened.SourceEvent.ID {
		t.Fatalf("merge = %#v / %v", merged, err)
	}
	closed, err := service.Govern(ctx, governMicroEventFixture(actorID, "close_event", merged.TargetEvent.ID,
		merged.TargetEvent.Version, 0, 0, 0, 0, 0, "close-a"))
	if err != nil || closed.SourceEvent.Status != "closed" {
		t.Fatalf("close = %#v / %v", closed, err)
	}
}

func TestMicroEventGovernanceResolvesSameEventAndDifferentEventReviews(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var actorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('event-reviewer@example.test','fixture','Event Reviewer','editor') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	repository, _ := NewMicroEventRepository(runtime)
	microEvents, _ := eventapplication.NewMicroEventService(repository)
	governanceRepository, _ := NewMicroEventGovernancePostgresRepository(runtime)
	governance, _ := eventapplication.NewMicroEventGovernanceService(governanceRepository)
	firstFixture := seedMicroEventAssignmentFixture(t, runtime, "review-base", "accepted")
	first, err := microEvents.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: firstFixture.familyID, DocumentMatchDecisionID: firstFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	secondFixture := seedMicroEventAssignmentFixture(t, runtime, "review-same", "accepted")
	reviewed, err := microEvents.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: secondFixture.familyID, DocumentMatchDecisionID: secondFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil || reviewed.Decision.Action != "review" || reviewed.Event.ID != first.Event.ID {
		t.Fatalf("same-event review = %#v / %v", reviewed, err)
	}
	accepted, err := governance.Govern(ctx, governMicroEventFixture(actorID, "same_event", reviewed.Event.ID,
		reviewed.Event.Version, reviewed.Decision.ID, secondFixture.familyID, 0, 0, 0, "same-review"))
	if err != nil || accepted.Feedback.ResultMembershipDecisionID <= 0 || accepted.SourceEvent.Status != "active" {
		t.Fatalf("same-event feedback = %#v / %v", accepted, err)
	}
	thirdFixture := seedMicroEventAssignmentFixture(t, runtime, "review-different", "accepted")
	thirdReview, err := microEvents.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: thirdFixture.familyID, DocumentMatchDecisionID: thirdFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil || thirdReview.Decision.Action != "review" {
		t.Fatalf("different-event review = %#v / %v", thirdReview, err)
	}
	different, err := governance.Govern(ctx, governMicroEventFixture(actorID, "different_event", thirdReview.Event.ID,
		thirdReview.Event.Version, thirdReview.Decision.ID, thirdFixture.familyID, 0, 0, 0, "different-review"))
	if err != nil || different.TargetEvent == nil || different.TargetEvent.ID == thirdReview.Event.ID ||
		different.Feedback.ResultMembershipDecisionID <= 0 {
		t.Fatalf("different-event feedback = %#v / %v", different, err)
	}
}

func setMicroEventFixtureAction(t *testing.T, runtime *database.Runtime, documentVersionID int64, action string) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE document_version_search_indexes SET action_keys=ARRAY[$1]
WHERE document_version_id=$2`, action, documentVersionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func governMicroEventFixture(actorID int64, action string, eventID, eventVersion, decisionID, familyID,
	memberVersion, targetID, targetVersion int64, key string) eventapplication.GovernMicroEventCommand {
	return eventapplication.GovernMicroEventCommand{ActorUserID: actorID, Action: action, MicroEventID: eventID,
		ExpectedEventVersion: eventVersion, MembershipDecisionID: decisionID, ContentFamilyID: familyID,
		ExpectedMemberVersion: memberVersion, TargetMicroEventID: targetID, ExpectedTargetEventVersion: targetVersion,
		ReasonCode: "editor_confirmed", Note: fmt.Sprintf("governance %s", action),
		GovernanceProfileVersion: eventapplication.CanonicalMicroEventGovernanceProfileVersion, IdempotencyKey: key}
}
