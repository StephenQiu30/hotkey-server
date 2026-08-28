//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestClaimEvidenceRepositoryPersistsQuotedRelationsAndLineageEvidenceState(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	first := seedMicroEventAssignmentFixture(t, runtime, "claim-evidence-first", "accepted")
	second := seedMicroEventAssignmentFixture(t, runtime, "claim-evidence-second", "accepted")
	microRepository, _ := NewMicroEventRepository(runtime)
	microService, _ := eventapplication.NewMicroEventService(microRepository)
	firstAssignment, err := microService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: first.familyID, DocumentMatchDecisionID: first.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	secondAssignment, err := microRepository.CommitMicroEventMembership(ctx, microEventCommitFixture(second, "join",
		firstAssignment.Event.ID, firstAssignment.Event.Version, strings.Repeat("8", 64), "claim-evidence-join"))
	if err != nil || secondAssignment.Event.ID != firstAssignment.Event.ID {
		t.Fatalf("second assignment = %#v / %v", secondAssignment, err)
	}
	storylineRepository, _ := NewStorylinePostgresRepository(runtime)
	storylineService, _ := eventapplication.NewStorylineService(storylineRepository)
	if _, err := storylineService.Assign(ctx, eventapplication.AssignMicroEventToStorylineCommand{MicroEventID: secondAssignment.Event.ID,
		MicroEventVersion: secondAssignment.Event.Version, RelationProfileVersion: eventapplication.CanonicalStorylineRelationProfileVersion}); err != nil {
		t.Fatalf("storyline assignment: %v", err)
	}

	actorID, firstSelectorID := attachClaimEvidenceQuoteFixture(t, runtime, first, "first")
	_, secondSelectorID := attachClaimEvidenceQuoteFixture(t, runtime, second, "second")
	if _, err := runtime.SQL.Exec(`INSERT INTO evidence_state_profiles
(algorithm_version,status,activated_by_user_id,activated_at) VALUES ($1,'active',$2,$3)`,
		eventapplication.CanonicalEvidenceStateAlgorithmVersion, actorID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	repository, _ := NewClaimEvidencePostgresRepository(runtime)
	service, _ := eventapplication.NewClaimEvidenceService(repository)
	now := time.Now().UTC().Truncate(time.Microsecond)
	currentEventVersion := secondAssignment.Event.Version
	firstResult, err := service.Record(ctx, manualClaimEvidenceCommand(firstAssignment.Event.ID, currentEventVersion,
		first.documentVersionID, firstSelectorID, actorID, "asserts", "claim-evidence-first", now))
	if err != nil {
		t.Fatalf("Record(first): %v", err)
	}
	var originalEvidenceBefore, documentBefore, observationBefore string
	if err := runtime.SQL.QueryRow(`SELECT row_to_json(evidence)::text,row_to_json(document_version)::text,row_to_json(observation)::text
FROM claim_evidence_versions AS evidence
JOIN document_versions AS document_version ON document_version.id=evidence.document_version_id
JOIN source_observations AS observation ON observation.id=document_version.source_observation_id
WHERE evidence.id=$1`, firstResult.Evidence.ID).
		Scan(&originalEvidenceBefore, &documentBefore, &observationBefore); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Record(ctx, manualClaimEvidenceCommand(firstAssignment.Event.ID, currentEventVersion,
		first.documentVersionID, firstSelectorID, actorID, "asserts", "claim-evidence-first", now))
	if err != nil || replayed.Evidence.ID != firstResult.Evidence.ID || replayed.Created {
		t.Fatalf("replay = %#v / %v", replayed, err)
	}
	_, correctedSelectorID := attachClaimEvidenceQuoteFixture(t, runtime, first, "first-corrected")
	corrected, err := service.Correct(ctx, eventapplication.CorrectClaimEvidenceCommand{
		OriginalClaimEvidenceVersionID: firstResult.Evidence.ID, ExpectedClaimVersion: firstResult.Claim.Version,
		ResultTextQuoteSelectorID: correctedSelectorID, ResultRelation: "mentions", ActorUserID: actorID,
		ReasonCode: "locator_and_relation_corrected", Note: "editor reviewed the exact archived body",
		IdempotencyKey: "claim-evidence-correction", DecisionAt: now,
	})
	if err != nil || corrected.Feedback.OriginalClaimEvidenceVersionID != firstResult.Evidence.ID ||
		corrected.Evidence.TextQuoteSelectorID != correctedSelectorID || corrected.Evidence.Relation != "mentions" {
		t.Fatalf("Correct() = %#v / %v", corrected, err)
	}
	var storedActor, expectedClaimVersion int64
	var storedReason, originalRelation, resultRelation string
	if err := runtime.SQL.QueryRow(`SELECT actor_user_id,expected_claim_version,reason_code,original_relation,result_relation
FROM claim_evidence_feedbacks WHERE id=$1`, corrected.Feedback.ID).
		Scan(&storedActor, &expectedClaimVersion, &storedReason, &originalRelation, &resultRelation); err != nil {
		t.Fatal(err)
	}
	if storedActor != actorID || expectedClaimVersion != firstResult.Claim.Version ||
		storedReason != "locator_and_relation_corrected" || originalRelation != "asserts" || resultRelation != "mentions" {
		t.Fatalf("correction audit = actor %d claim v%d reason %q relation %s->%s",
			storedActor, expectedClaimVersion, storedReason, originalRelation, resultRelation)
	}
	var originalEvidenceAfter, documentAfter, observationAfter string
	if err := runtime.SQL.QueryRow(`SELECT row_to_json(evidence)::text,row_to_json(document_version)::text,row_to_json(observation)::text
FROM claim_evidence_versions AS evidence
JOIN document_versions AS document_version ON document_version.id=evidence.document_version_id
JOIN source_observations AS observation ON observation.id=document_version.source_observation_id
WHERE evidence.id=$1`, firstResult.Evidence.ID).
		Scan(&originalEvidenceAfter, &documentAfter, &observationAfter); err != nil {
		t.Fatal(err)
	}
	if originalEvidenceAfter != originalEvidenceBefore || documentAfter != documentBefore || observationAfter != observationBefore {
		t.Fatalf("correction rewrote immutable facts: evidence changed=%t document changed=%t observation changed=%t",
			originalEvidenceAfter != originalEvidenceBefore, documentAfter != documentBefore, observationAfter != observationBefore)
	}
	correctionReplay, err := service.Correct(ctx, eventapplication.CorrectClaimEvidenceCommand{
		OriginalClaimEvidenceVersionID: firstResult.Evidence.ID, ExpectedClaimVersion: firstResult.Claim.Version,
		ResultTextQuoteSelectorID: correctedSelectorID, ResultRelation: "mentions", ActorUserID: actorID,
		ReasonCode: "locator_and_relation_corrected", Note: "editor reviewed the exact archived body",
		IdempotencyKey: "claim-evidence-correction", DecisionAt: now,
	})
	if err != nil || correctionReplay.Feedback.ID != corrected.Feedback.ID || correctionReplay.Created {
		t.Fatalf("correction replay = %#v / %v", correctionReplay, err)
	}
	if _, err := service.Correct(ctx, eventapplication.CorrectClaimEvidenceCommand{
		OriginalClaimEvidenceVersionID: firstResult.Evidence.ID, ExpectedClaimVersion: firstResult.Claim.Version,
		ResultTextQuoteSelectorID: correctedSelectorID, ResultRelation: "attributes_to", ActorUserID: actorID,
		ReasonCode: "branch_from_superseded_version", IdempotencyKey: "claim-evidence-invalid-branch", DecisionAt: now,
	}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("correction from superseded evidence error = %v", err)
	}
	secondResult, err := service.Record(ctx, manualClaimEvidenceCommand(firstAssignment.Event.ID, currentEventVersion,
		second.documentVersionID, secondSelectorID, actorID, "contradicts", "claim-evidence-second", now))
	if err != nil {
		t.Fatalf("Record(second): %v", err)
	}
	state, err := service.CalculateState(ctx, eventapplication.CalculateEvidenceStateCommand{MicroEventID: firstAssignment.Event.ID,
		ExpectedEventVersion: currentEventVersion, AlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion,
		CalculatedAt: now})
	if err != nil || state.Snapshot.State != "conflicting_reports" || state.Snapshot.IndependentOriginCount != 2 ||
		len(state.Snapshot.ClaimEvidenceVersionIDs) != 2 ||
		!containsExactlyEvidenceIDs(state.Snapshot.ClaimEvidenceVersionIDs, corrected.Evidence.ID, secondResult.Evidence.ID) {
		t.Fatalf("CalculateState() = %#v / %v", state, err)
	}
	if _, err := repository.CommitEvidenceStateSnapshot(ctx, eventapplication.CommitEvidenceStateSnapshotCommand{
		MicroEventID: firstAssignment.Event.ID, EventVersion: currentEventVersion, ProfileID: state.Snapshot.ProfileID,
		AlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion, EvidenceSetHash: strings.Repeat("e", 64),
		State: "single_origin", IndependentOriginCount: 1, ReasonCodes: []string{"single_independent_origin"},
		ClaimEvidenceVersionIDs: []int64{firstResult.Evidence.ID}, CalculatedAt: now,
	}); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("snapshot accepted a superseded and incomplete Evidence set: %v", err)
	}
	replayedState, err := service.CalculateState(ctx, eventapplication.CalculateEvidenceStateCommand{MicroEventID: firstAssignment.Event.ID,
		ExpectedEventVersion: currentEventVersion, AlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion,
		CalculatedAt: now.Add(time.Minute)})
	if err != nil || replayedState.Snapshot.ID != state.Snapshot.ID || replayedState.Snapshot.Created {
		t.Fatalf("state replay = %#v / %v", replayedState, err)
	}
	summaryRepository, _ := NewEvidenceSummaryPostgresRepository(runtime)
	summaryService, _ := eventapplication.NewEvidenceSummaryService(summaryRepository)
	if _, err := summaryService.Publish(ctx, eventapplication.PublishEvidenceSummaryCommand{MicroEventID: firstAssignment.Event.ID,
		ExpectedEventVersion: currentEventVersion, SummaryProfileVersion: "superseded-evidence-summary-v2",
		IdempotencyKey: "superseded-claim-evidence-summary", CreatedAt: now,
		Sentences: []eventapplication.EvidenceSummarySentenceInputDTO{{Text: "汇总状态不能替代逐句当前证据。",
			ClaimEvidenceVersionIDs: []int64{firstResult.Evidence.ID}, DecisionOrigin: "manual", ActorUserID: &actorID}}}); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("summary approved from superseded ClaimEvidence: %v", err)
	}
	summary, err := summaryService.Publish(ctx, eventapplication.PublishEvidenceSummaryCommand{MicroEventID: firstAssignment.Event.ID,
		ExpectedEventVersion: currentEventVersion, SummaryProfileVersion: "evidence-summary-v2",
		IdempotencyKey: "claim-evidence-summary", CreatedAt: now,
		Sentences: []eventapplication.EvidenceSummarySentenceInputDTO{
			{Text: "报道对该事件存在相反表述。", ClaimEvidenceVersionIDs: []int64{corrected.Evidence.ID, secondResult.Evidence.ID}, DecisionOrigin: "manual", ActorUserID: &actorID},
			{Text: "编者提示：请继续关注原发布者更新。", EditorialNote: true, DecisionOrigin: "manual", ActorUserID: &actorID},
		}})
	if err != nil || len(summary.Summary.Sentences) != 2 ||
		!containsExactlyEvidenceIDs(summary.Summary.Sentences[0].ClaimEvidenceVersionIDs, corrected.Evidence.ID, secondResult.Evidence.ID) ||
		!summary.Summary.Sentences[1].EditorialNote || len(summary.Summary.Sentences[1].ClaimEvidenceVersionIDs) != 0 {
		t.Fatalf("Publish(summary) = %#v / %v", summary, err)
	}
	summaryReplay, err := summaryService.Publish(ctx, eventapplication.PublishEvidenceSummaryCommand{MicroEventID: firstAssignment.Event.ID,
		ExpectedEventVersion: currentEventVersion, SummaryProfileVersion: "evidence-summary-v2",
		IdempotencyKey: "claim-evidence-summary", CreatedAt: now,
		Sentences: []eventapplication.EvidenceSummarySentenceInputDTO{
			{Text: "报道对该事件存在相反表述。", ClaimEvidenceVersionIDs: []int64{corrected.Evidence.ID, secondResult.Evidence.ID}, DecisionOrigin: "manual", ActorUserID: &actorID},
			{Text: "编者提示：请继续关注原发布者更新。", EditorialNote: true, DecisionOrigin: "manual", ActorUserID: &actorID},
		}})
	if err != nil || summaryReplay.Summary.ID != summary.Summary.ID || summaryReplay.Summary.Created {
		t.Fatalf("summary replay = %#v / %v", summaryReplay, err)
	}
	queryRepository, _ := NewMicroEventQueryPostgresRepository(runtime)
	queryService, _ := eventapplication.NewMicroEventQueryService(queryRepository)
	page, err := queryService.List(ctx, eventapplication.MicroEventListQuery{Limit: 10, Statuses: []string{"active"}})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != firstAssignment.Event.ID ||
		page.Items[0].LatestEvidenceState == nil || page.Items[0].LatestEvidenceState.State != "conflicting_reports" ||
		page.Items[0].ContentFamilyCount != 2 {
		t.Fatalf("List(v2) = %#v / %v", page, err)
	}
	detail, err := queryService.Get(ctx, firstAssignment.Event.ID)
	if err != nil || detail.Storyline == nil || detail.DocumentCount != 2 || len(detail.Members) != 2 ||
		detail.Members[0].ContentFamilyID <= 0 || detail.Members[0].MembershipDecisionID <= 0 || detail.Members[0].Version <= 0 ||
		detail.Members[1].ContentFamilyID <= 0 || detail.Members[1].MembershipDecisionID <= 0 || detail.Members[1].Version <= 0 {
		t.Fatalf("Get(v2) = %#v / %v", detail, err)
	}
	evidenceQueryNow := now
	queryRepository.now = func() time.Time { return evidenceQueryNow }
	evidencePage, err := queryService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{MicroEventID: firstAssignment.Event.ID, Limit: 10})
	if err != nil || len(evidencePage.Items) != 3 || evidencePage.Items[0].ClaimVersion != 1 ||
		evidencePage.Items[0].Availability != "ready" || evidencePage.Items[0].ExactQuote == nil {
		t.Fatalf("Evidence(v2) = %#v / %v", evidencePage, err)
	}
	insertClaimEvidenceQuoteDeny(t, runtime, first, now)
	evidenceQueryNow = now.Add(time.Minute)
	evidenceAfterDeny, err := queryService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{MicroEventID: firstAssignment.Event.ID, Limit: 10})
	if err != nil || evidenceAfterDeny.Items[0].Availability != "rights_unavailable" || evidenceAfterDeny.Items[0].ExactQuote != nil ||
		evidenceAfterDeny.Items[1].Availability != "rights_unavailable" || evidenceAfterDeny.Items[2].Availability != "ready" {
		t.Fatalf("Evidence(after deny) = %#v / %v", evidenceAfterDeny, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE claim_evidence_versions SET relation='mentions' WHERE id=$1`, firstResult.Evidence.ID); err == nil {
		t.Fatal("append-only claim evidence accepted update")
	} else {
		assertMicroEventGovernanceSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`UPDATE claim_evidence_feedbacks SET note='tampered' WHERE id=$1`, corrected.Feedback.ID); err == nil {
		t.Fatal("append-only claim evidence feedback accepted update")
	} else {
		assertMicroEventGovernanceSQLState(t, err, "23514")
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM claim_evidence_feedbacks WHERE id=$1`, corrected.Feedback.ID); err == nil {
		t.Fatal("append-only claim evidence feedback accepted delete")
	} else {
		assertMicroEventGovernanceSQLState(t, err, "23514")
	}
}

func TestMicroEventEvidenceCursorIsSignedBoundExpiringAndSnapshotStable(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	fixture := seedMicroEventAssignmentFixture(t, runtime, "evidence-cursor", "accepted")
	microRepository, _ := NewMicroEventRepository(runtime)
	microService, _ := eventapplication.NewMicroEventService(microRepository)
	assignment, err := microService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: fixture.familyID, DocumentMatchDecisionID: fixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	actorID, firstSelectorID := attachClaimEvidenceQuoteFixture(t, runtime, fixture, "cursor-first")
	claimRepository, _ := NewClaimEvidencePostgresRepository(runtime)
	claimService, _ := eventapplication.NewClaimEvidenceService(claimRepository)
	base := time.Now().UTC().Truncate(time.Microsecond)
	firstEvidence, err := claimService.Record(ctx, manualClaimEvidenceCommand(assignment.Event.ID, assignment.Event.Version,
		fixture.documentVersionID, firstSelectorID, actorID, "asserts", "evidence-cursor-first", base.Add(-3*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	_, secondSelectorID := attachClaimEvidenceQuoteFixture(t, runtime, fixture, "cursor-second")
	secondEvidence, err := claimService.Correct(ctx, eventapplication.CorrectClaimEvidenceCommand{
		OriginalClaimEvidenceVersionID: firstEvidence.Evidence.ID, ExpectedClaimVersion: firstEvidence.Claim.Version,
		ResultTextQuoteSelectorID: secondSelectorID, ResultRelation: "mentions", ActorUserID: actorID,
		ReasonCode: "cursor_second_version", IdempotencyKey: "evidence-cursor-second", DecisionAt: base.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	codec, err := pagination.NewCodec("micro-event-evidence-cursor-test-secret-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	queryRepository, err := NewMicroEventQueryPostgresRepositoryWithCursorCodec(runtime, codec)
	if err != nil {
		t.Fatal(err)
	}
	snapshotAt := base
	queryRepository.now = func() time.Time { return snapshotAt }
	queryService, _ := eventapplication.NewMicroEventQueryService(queryRepository)
	firstPage, err := queryService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{MicroEventID: assignment.Event.ID, Limit: 1})
	if err != nil || len(firstPage.Items) != 1 || firstPage.Items[0].ID != firstEvidence.Evidence.ID || firstPage.NextCursor == "" {
		t.Fatalf("first evidence page = %#v / %v", firstPage, err)
	}
	if _, err := strconv.ParseInt(firstPage.NextCursor, 10, 64); err == nil {
		t.Fatalf("evidence cursor is a naked integer: %q", firstPage.NextCursor)
	}
	tampered := "A" + firstPage.NextCursor[1:]
	if tampered == firstPage.NextCursor {
		tampered = "B" + firstPage.NextCursor[1:]
	}
	if _, err := queryService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{
		MicroEventID: assignment.Event.ID, Limit: 1, Cursor: tampered,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("tampered evidence cursor error = %v, want invalid input", err)
	}
	if _, err := queryService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{
		MicroEventID: assignment.Event.ID + 1, Limit: 1, Cursor: firstPage.NextCursor,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-event evidence cursor error = %v, want invalid input", err)
	}

	_, thirdSelectorID := attachClaimEvidenceQuoteFixture(t, runtime, fixture, "cursor-third-unique")
	var currentClaimVersion int64
	if err := runtime.SQL.QueryRow(`SELECT version FROM claims WHERE id=$1`, firstEvidence.Claim.ID).Scan(&currentClaimVersion); err != nil {
		t.Fatal(err)
	}
	thirdEvidence, err := claimService.Correct(ctx, eventapplication.CorrectClaimEvidenceCommand{
		OriginalClaimEvidenceVersionID: secondEvidence.Evidence.ID, ExpectedClaimVersion: currentClaimVersion,
		ResultTextQuoteSelectorID: thirdSelectorID, ResultRelation: "attributes_to", ActorUserID: actorID,
		ReasonCode: "cursor_concurrent_version", IdempotencyKey: "evidence-cursor-third", DecisionAt: snapshotAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := queryService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{
		MicroEventID: assignment.Event.ID, Limit: 1, Cursor: firstPage.NextCursor,
	})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != secondEvidence.Evidence.ID ||
		secondPage.Items[0].ID == thirdEvidence.Evidence.ID || secondPage.NextCursor != "" {
		t.Fatalf("snapshot-stable second evidence page = %#v / %v; concurrent=%d", secondPage, err, thirdEvidence.Evidence.ID)
	}

	expiringCodec, err := pagination.NewCodec("expiring-micro-event-evidence-cursor-secret-32b", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	expiringRepository, _ := NewMicroEventQueryPostgresRepositoryWithCursorCodec(runtime, expiringCodec)
	expiringRepository.now = func() time.Time { return snapshotAt.Add(2 * time.Second) }
	expiringService, _ := eventapplication.NewMicroEventQueryService(expiringRepository)
	expiringPage, err := expiringService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{MicroEventID: assignment.Event.ID, Limit: 1})
	if err != nil || expiringPage.NextCursor == "" {
		t.Fatalf("expiring first page = %#v / %v", expiringPage, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := expiringService.Evidence(ctx, eventapplication.MicroEventEvidenceQuery{
		MicroEventID: assignment.Event.ID, Limit: 1, Cursor: expiringPage.NextCursor,
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired evidence cursor error = %v, want invalid input", err)
	}
}

func containsExactlyEvidenceIDs(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[int64]int, len(got))
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] != 1 {
			return false
		}
	}
	return true
}

func insertClaimEvidenceQuoteDeny(t *testing.T, runtime *database.Runtime, fixture microEventAssignmentFixture, now time.Time) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES ($1,$2,$3,1,'source_endpoint',$4,300,'fixture quote revoked','document_version',$5,$6,
'quote','deny',ARRAY['fixture_revoke'],'fixture',$7,$7)`, 940000+fixture.documentVersionID,
		fixture.sourceID, 950000+fixture.documentVersionID, fmt.Sprintf("source:%d", fixture.sourceID),
		fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64), now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func manualClaimEvidenceCommand(eventID, eventVersion, documentVersionID, selectorID, actorID int64, relation, key string, at time.Time) eventapplication.RecordClaimEvidenceCommand {
	return eventapplication.RecordClaimEvidenceCommand{MicroEventID: eventID, ExpectedEventVersion: eventVersion,
		DocumentVersionID: documentVersionID, TextQuoteSelectorID: selectorID, Subject: "共享主体", Predicate: "发布", Object: "同一事件",
		Qualifiers: []eventapplication.ClaimQualifierDTO{{Key: "time", Value: "today"}}, Relation: relation,
		ExtractionSchemaVersion: eventapplication.CanonicalClaimExtractionSchemaVersion, Origin: "manual", ActorUserID: &actorID,
		IdempotencyKey: key, DecisionAt: at}
}

func attachClaimEvidenceQuoteFixture(t *testing.T, runtime *database.Runtime, fixture microEventAssignmentFixture, suffix string) (int64, int64) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var actorID, quoteDecisionID, selectorID int64
	if err := transaction.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'fixture','Claim Evidence Editor','editor') RETURNING id`, "claim-evidence-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`SELECT COALESCE(max(id),0) FROM source_rights_decisions
WHERE source_connection_id=$1 AND subject_type='document_version' AND subject_key=$2 AND input_digest=$3 AND action='quote'`,
		fixture.sourceID, fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64)).Scan(&quoteDecisionID); err != nil {
		t.Fatal(err)
	}
	if quoteDecisionID == 0 {
		if err := transaction.QueryRow(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES ($1,$2,$3,1,'source_endpoint',$4,200,'fixture quote','document_version',$5,$6,
'quote','allow',ARRAY['fixture'],'fixture',$7,$7) RETURNING id`, 900000+fixture.documentVersionID,
			fixture.sourceID, 910000+fixture.documentVersionID, fmt.Sprintf("source:%d", fixture.sourceID),
			fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64), now.Add(-time.Hour)).Scan(&quoteDecisionID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := transaction.Exec(`UPDATE document_versions SET lifecycle_state='readable' WHERE id=$1`, fixture.documentVersionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`INSERT INTO document_text_quote_selectors (
source_connection_id,document_version_id,plaintext_artifact_id,markdown_artifact_id,quote_rights_decision_id,retain_rights_decision_id,
exact_quote,prefix,suffix,utf8_byte_start,utf8_byte_end,quote_sha256,plaintext_sha256,normalization_version,selector_version,
anchor_map_sha256,retention_until,created_at) VALUES ($1,$2,$3,$4,$5,$6,'同一事件','共享主体发布','',18,30,$7,$8,
'nfc-lf-collapse-space-v1','w3c-text-quote-position-nfc-utf8-v1',$9,$10,$11) RETURNING id`, fixture.sourceID,
		fixture.documentVersionID, 920000+fixture.documentVersionID, 930000+fixture.documentVersionID, quoteDecisionID,
		fixture.retainDecisionID, strings.Repeat(fmt.Sprintf("%x", len(suffix)%15+1), 64), strings.Repeat("d", 64), strings.Repeat("b", 64),
		now.Add(24*time.Hour), now).Scan(&selectorID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	return actorID, selectorID
}
