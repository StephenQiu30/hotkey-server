//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
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

	wantActions := map[string]bool{"move_member": false, "reopen_event": false, "split_event": false, "merge_events": false, "close_event": false}
	rows, err := runtime.SQL.Query(`SELECT feedback_type,actor_user_id,reason_code,original_event_version,
result_event_version,governance_profile_version FROM micro_event_feedbacks WHERE actor_user_id=$1`, actorID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var action, reason, profile string
		var storedActor, beforeVersion, afterVersion int64
		if err := rows.Scan(&action, &storedActor, &reason, &beforeVersion, &afterVersion, &profile); err != nil {
			t.Fatal(err)
		}
		if _, ok := wantActions[action]; !ok || storedActor != actorID || reason != "editor_confirmed" ||
			beforeVersion <= 0 || afterVersion <= beforeVersion || profile != eventapplication.CanonicalMicroEventGovernanceProfileVersion {
			t.Fatalf("invalid governance audit %s/%d/%s/v%d->v%d/%s", action, storedActor, reason, beforeVersion, afterVersion, profile)
		}
		wantActions[action] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for action, found := range wantActions {
		if !found {
			t.Errorf("missing immutable governance audit for %s", action)
		}
	}
}

func TestMicroEventGovernanceRejectsUnauthorizedStaleAndConflictingCommandsAndKeepsWithdrawAuditImmutable(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var editorID, viewerID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('withdraw-editor@example.test','fixture','Withdraw Editor','editor') RETURNING id`).Scan(&editorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('withdraw-viewer@example.test','fixture','Withdraw Viewer','viewer') RETURNING id`).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "governance-withdraw", "accepted")
	events, _ := NewMicroEventRepository(runtime)
	assignmentService, _ := eventapplication.NewMicroEventService(events)
	assigned, err := assignmentService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: fixture.familyID, DocumentMatchDecisionID: fixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	var documentBefore, observationBefore string
	if err := runtime.SQL.QueryRow(`SELECT row_to_json(document_version)::text,row_to_json(observation)::text
FROM document_versions AS document_version
JOIN source_observations AS observation ON observation.id=document_version.source_observation_id
WHERE document_version.id=$1`, fixture.documentVersionID).Scan(&documentBefore, &observationBefore); err != nil {
		t.Fatal(err)
	}
	repository, _ := NewMicroEventGovernancePostgresRepository(runtime)
	governance, _ := eventapplication.NewMicroEventGovernanceService(repository)
	unauthorized := governMicroEventFixture(viewerID, "withdraw", assigned.Event.ID, assigned.Event.Version,
		assigned.Decision.ID, fixture.familyID, 1, 0, 0, "withdraw-viewer")
	if _, err := governance.Govern(ctx, unauthorized); !errors.Is(err, eventapplication.ErrMicroEventGovernanceForbidden) {
		t.Fatalf("viewer governance error = %v, want forbidden", err)
	}
	stale := governMicroEventFixture(editorID, "withdraw", assigned.Event.ID, assigned.Event.Version+1,
		assigned.Decision.ID, fixture.familyID, 1, 0, 0, "withdraw-stale")
	if _, err := governance.Govern(ctx, stale); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale governance error = %v, want conflict", err)
	}
	command := governMicroEventFixture(editorID, "withdraw", assigned.Event.ID, assigned.Event.Version,
		assigned.Decision.ID, fixture.familyID, 1, 0, 0, "withdraw-member")
	withdrawn, err := governance.Govern(ctx, command)
	if err != nil || withdrawn.SourceEvent.Status != "closed" || withdrawn.SourceEvent.Version != assigned.Event.Version+1 ||
		withdrawn.Feedback.ActorUserID != editorID || withdrawn.Feedback.ReasonCode != "editor_confirmed" ||
		withdrawn.Feedback.OriginalEventVersion != assigned.Event.Version || withdrawn.Feedback.ResultEventVersion != assigned.Event.Version+1 {
		t.Fatalf("withdraw governance = %#v / %v", withdrawn, err)
	}
	replayed, err := governance.Govern(ctx, command)
	if err != nil || replayed.Feedback.ID != withdrawn.Feedback.ID {
		t.Fatalf("withdraw replay = %#v / %v", replayed, err)
	}
	conflict := command
	conflict.ReasonCode = "different_reason"
	if _, err := governance.Govern(ctx, conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("idempotency conflict error = %v, want conflict", err)
	}
	var feedbackCount, memberCount, activeMemberCount, memberVersion int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM micro_event_feedbacks WHERE idempotency_key='withdraw-member'`).Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*),count(*) FILTER (WHERE active),max(version) FROM micro_event_members
WHERE micro_event_id=$1 AND content_family_id=$2`, assigned.Event.ID, fixture.familyID).
		Scan(&memberCount, &activeMemberCount, &memberVersion); err != nil {
		t.Fatal(err)
	}
	if feedbackCount != 1 || memberCount != 1 || activeMemberCount != 0 || memberVersion != 2 {
		t.Fatalf("withdraw facts feedback/member/active/version = %d/%d/%d/%d",
			feedbackCount, memberCount, activeMemberCount, memberVersion)
	}
	var documentAfter, observationAfter string
	if err := runtime.SQL.QueryRow(`SELECT row_to_json(document_version)::text,row_to_json(observation)::text
FROM document_versions AS document_version
JOIN source_observations AS observation ON observation.id=document_version.source_observation_id
WHERE document_version.id=$1`, fixture.documentVersionID).Scan(&documentAfter, &observationAfter); err != nil {
		t.Fatal(err)
	}
	if documentAfter != documentBefore || observationAfter != observationBefore {
		t.Fatalf("governance rewrote immutable source facts: document changed=%t observation changed=%t",
			documentAfter != documentBefore, observationAfter != observationBefore)
	}
	if _, err := runtime.SQL.Exec(`UPDATE micro_event_feedbacks SET note='tampered' WHERE id=$1`, withdrawn.Feedback.ID); err == nil {
		t.Fatal("governance audit update succeeded, want append-only rejection")
	} else {
		assertMicroEventGovernanceSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM micro_event_feedbacks WHERE id=$1`, withdrawn.Feedback.ID); err == nil {
		t.Fatal("governance audit delete succeeded, want append-only rejection")
	} else {
		assertMicroEventGovernanceSQLState(t, err, "23514")
	}
}

func assertMicroEventGovernanceSQLState(t *testing.T, err error, state string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != state {
		t.Fatalf("database error = %v, want SQLSTATE %s", err, state)
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
	if err != nil || accepted.Feedback.ResultMembershipDecisionID <= 0 || accepted.SourceEvent.Status != "active" ||
		accepted.Feedback.ActorUserID != actorID || accepted.Feedback.ReasonCode != "editor_confirmed" ||
		accepted.Feedback.OriginalEventVersion != reviewed.Event.Version ||
		accepted.Feedback.ResultEventVersion <= accepted.Feedback.OriginalEventVersion {
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
		different.Feedback.ResultMembershipDecisionID <= 0 || different.Feedback.ActorUserID != actorID ||
		different.Feedback.ReasonCode != "editor_confirmed" || different.Feedback.OriginalEventVersion != thirdReview.Event.Version ||
		different.Feedback.ResultEventVersion <= different.Feedback.OriginalEventVersion {
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
