//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

type reportEvidenceFixture struct {
	report                            domain.Report
	actorID, sourceID                 int64
	documentVersionID, selectorID     int64
	microEventID, summaryID           int64
	microEventUpdateID                int64
	claimEvidenceVersionID            int64
	quoteDecisionID, retainDecisionID int64
}

func TestReportRepositoryFreezesExactSentenceCitationsAndPublishedVersion(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedReportEvidenceFixture(t, runtime)
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, fixture.report); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidatePublication(ctx, fixture.report); err != nil {
		t.Fatalf("valid cited draft rejected: %v", err)
	}
	loaded, err := repository.Get(ctx, fixture.report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Items) != 1 || len(loaded.Items[0].Sentences) != 1 ||
		len(loaded.Items[0].Sentences[0].ClaimEvidenceVersionIDs) != 1 ||
		loaded.Items[0].Sentences[0].ClaimEvidenceVersionIDs[0] != fixture.claimEvidenceVersionID {
		t.Fatalf("loaded cited report = %#v", loaded)
	}
	pending, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: loaded.ID, ExpectedVersion: loaded.Version,
		ActorID: fixture.actorID, From: domain.ReportDraft, To: domain.ReportPendingApproval})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidatePublication(ctx, pending); err != nil {
		t.Fatal(err)
	}
	published, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: pending.ID, ExpectedVersion: pending.Version,
		ActorID: fixture.actorID, From: domain.ReportPendingApproval, To: domain.ReportPublished})
	if err != nil {
		t.Fatal(err)
	}
	if !published.Frozen {
		t.Fatalf("published report = %#v", published)
	}
	if err := repository.Save(ctx, fixture.report); !errors.Is(err, sharedrepository.ErrImmutable) {
		t.Fatalf("save stale draft error = %v, want ErrImmutable", err)
	}
}

func TestReportRevisionApprovalIsOptimisticAuditedAndRegenerationPreservesApprovedSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedReportEvidenceFixture(t, runtime)
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, fixture.report); err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: fixture.report.ID, ExpectedVersion: 1,
		ActorID: fixture.actorID, From: domain.ReportDraft, To: domain.ReportPendingApproval})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Version != 2 || pending.Status != domain.ReportPendingApproval {
		t.Fatalf("pending revision = %#v", pending)
	}
	if _, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: fixture.report.ID, ExpectedVersion: 1,
		ActorID: fixture.actorID, From: domain.ReportPendingApproval, To: domain.ReportPublished}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale approval error = %v, want ErrConflict", err)
	}
	if err := repository.ValidatePublication(ctx, pending); err != nil {
		t.Fatal(err)
	}
	approved, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: fixture.report.ID, ExpectedVersion: 2,
		ActorID: fixture.actorID, From: domain.ReportPendingApproval, To: domain.ReportPublished})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Version != 3 || approved.Status != domain.ReportPublished || !approved.Frozen {
		t.Fatalf("approved revision = %#v", approved)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE reports SET body='tampered' WHERE id=$1`, approved.ID); err == nil {
		t.Fatal("approved report body update succeeded")
	}
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM report_items WHERE report_id=$1`, approved.ID); err == nil {
		t.Fatal("approved report item deletion succeeded")
	}

	regenerated := cloneCitedReport(fixture.report)
	regenerated.ID, regenerated.Version, regenerated.VersionNo = 1, 1, 2
	regenerated.Title = "重新生成日报"
	regenerated.InputSnapshotHash = domain.ComputeInputSnapshotHash(regenerated)
	regenerated, err = repository.Create(ctx, regenerated)
	if err != nil {
		t.Fatal(err)
	}
	if regenerated.ID == approved.ID || regenerated.VersionNo != 2 || regenerated.Status != domain.ReportDraft {
		t.Fatalf("regenerated revision = %#v", regenerated)
	}
	original, err := repository.Get(ctx, approved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if original.Status != domain.ReportPublished || original.Title != fixture.report.Title ||
		len(original.Items) != 1 || len(original.Items[0].Sentences) != 1 ||
		original.Items[0].Sentences[0].ClaimEvidenceVersionIDs[0] != fixture.claimEvidenceVersionID {
		t.Fatalf("approved revision changed after regeneration: %#v", original)
	}
	latest, err := repository.FindByPeriod(ctx, fixture.report.Type, fixture.report.MonitorID, fixture.report.Period.Start, fixture.report.Period.End)
	if err != nil || latest.ID != regenerated.ID {
		t.Fatalf("latest revision = %#v/%v", latest, err)
	}
	var transitions int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM report_revision_transitions WHERE report_id=$1`, approved.ID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if transitions != 2 {
		t.Fatalf("revision transitions = %d, want 2", transitions)
	}
}

func TestReportRevisionConcurrentApprovalHasOneWinner(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedReportEvidenceFixture(t, runtime)
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, fixture.report); err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: fixture.report.ID, ExpectedVersion: 1,
		ActorID: fixture.actorID, From: domain.ReportDraft, To: domain.ReportPendingApproval})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			<-start
			_, transitionErr := repository.Transition(ctx, domain.RevisionTransition{ReportID: pending.ID, ExpectedVersion: pending.Version,
				ActorID: fixture.actorID, From: domain.ReportPendingApproval, To: domain.ReportPublished})
			results <- transitionErr
		}()
	}
	close(start)
	winners, conflicts := 0, 0
	for index := 0; index < 2; index++ {
		result := <-results
		switch {
		case result == nil:
			winners++
		case errors.Is(result, sharedrepository.ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent approval error = %v", result)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("concurrent approvals winners=%d conflicts=%d", winners, conflicts)
	}
}

func TestReportRevisionRejectionIsFrozenAndAudited(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedReportEvidenceFixture(t, runtime)
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, fixture.report); err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: fixture.report.ID, ExpectedVersion: 1,
		ActorID: fixture.actorID, From: domain.ReportDraft, To: domain.ReportPendingApproval})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := repository.Transition(ctx, domain.RevisionTransition{ReportID: pending.ID, ExpectedVersion: pending.Version,
		ActorID: fixture.actorID, From: domain.ReportPendingApproval, To: domain.ReportRejected, ReasonCode: "insufficient_context"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != domain.ReportRejected || rejected.Frozen || rejected.ReviewReason != "insufficient_context" ||
		rejected.ReviewedBy == nil || *rejected.ReviewedBy != fixture.actorID {
		t.Fatalf("rejected revision = %#v", rejected)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE reports SET title='tampered',version=version+1 WHERE id=$1`, rejected.ID); err == nil {
		t.Fatal("rejected report update succeeded")
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE report_revision_transitions SET reason_code='tampered' WHERE report_id=$1`, rejected.ID); err == nil {
		t.Fatal("revision transition audit update succeeded")
	}
}

func TestReportRepositoryRejectsUnsafeDraftBeforeAnyWrite(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedReportEvidenceFixture(t, runtime)
	unsafe := cloneCitedReport(fixture.report)
	unsafe.Items[0].Sentences[0].Text = `<img src=x onerror=alert(1)>`
	if err := NewRepository(runtime).Save(ctx, unsafe); !errors.Is(err, domain.ErrUnsafeContent) {
		t.Fatalf("unsafe draft error = %v, want ErrUnsafeContent", err)
	}
	var reports, reportItems int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM reports),(SELECT count(*) FROM report_items)`).
		Scan(&reports, &reportItems); err != nil {
		t.Fatal(err)
	}
	if reports != 0 || reportItems != 0 {
		t.Fatalf("unsafe draft side effects = reports %d items %d", reports, reportItems)
	}
}

func TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedReportEvidenceFixture(t, runtime)
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, fixture.report); err != nil {
		t.Fatal(err)
	}

	missingCitation := fixture.report
	missingCitation.Items = append([]domain.Item(nil), fixture.report.Items...)
	missingCitation.Items[0].Sentences = append([]domain.Sentence(nil), fixture.report.Items[0].Sentences...)
	missingCitation.Items[0].Sentences[0].ClaimEvidenceVersionIDs = nil
	if err := repository.ValidatePublication(ctx, missingCitation); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("missing sentence citation error = %v, want ErrEvidenceInvalid", err)
	}

	hashMismatch := cloneCitedReport(fixture.report)
	hashMismatch.Items[0].EvidenceSetHash = strings.Repeat("9", 64)
	hashMismatch = saveDraftMutation(t, ctx, repository, hashMismatch)
	if err := repository.ValidatePublication(ctx, hashMismatch); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("evidence hash mismatch error = %v, want ErrEvidenceInvalid", err)
	}
	fixture.report = saveDraftMutation(t, ctx, repository, fixture.report)

	textMismatch := cloneCitedReport(fixture.report)
	textMismatch.Items[0].Sentences[0].Text = "被篡改的事实句"
	textMismatch = saveDraftMutation(t, ctx, repository, textMismatch)
	if err := repository.ValidatePublication(ctx, textMismatch); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("source sentence mismatch error = %v, want ErrEvidenceInvalid", err)
	}
	fixture.report = saveDraftMutation(t, ctx, repository, fixture.report)

	setClaimEvidenceCapturedAt(t, runtime, fixture.claimEvidenceVersionID, fixture.report.Period.Start.Add(-time.Second))
	if err := repository.ValidatePublication(ctx, fixture.report); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("out-of-window citation error = %v, want ErrEvidenceInvalid", err)
	}
	setClaimEvidenceCapturedAt(t, runtime, fixture.claimEvidenceVersionID, fixture.report.Period.Start.Add(time.Hour))
	if err := repository.ValidatePublication(ctx, fixture.report); err != nil {
		t.Fatalf("restored cited draft rejected: %v", err)
	}

	insertRightsDeny(t, runtime, fixture)
	if err := repository.ValidatePublication(ctx, fixture.report); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("revoked quote rights error = %v, want ErrEvidenceInvalid", err)
	}
}

func cloneCitedReport(report domain.Report) domain.Report {
	result := report
	result.Items = append([]domain.Item(nil), report.Items...)
	for itemIndex := range result.Items {
		result.Items[itemIndex].Sentences = append([]domain.Sentence(nil), report.Items[itemIndex].Sentences...)
		for sentenceIndex := range result.Items[itemIndex].Sentences {
			result.Items[itemIndex].Sentences[sentenceIndex].ClaimEvidenceVersionIDs = append([]int64(nil),
				report.Items[itemIndex].Sentences[sentenceIndex].ClaimEvidenceVersionIDs...)
		}
	}
	return result
}

func saveDraftMutation(t *testing.T, ctx context.Context, repository *Repository, report domain.Report) domain.Report {
	t.Helper()
	current, err := repository.Get(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	report.Version = current.Version + 1
	report.Status, report.Frozen = domain.ReportDraft, false
	report.InputSnapshotHash = domain.ComputeInputSnapshotHash(report)
	if err := repository.Save(ctx, report); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.Get(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func seedReportEvidenceFixture(t *testing.T, runtime *database.Runtime) reportEvidenceFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	period, err := domain.PeriodFor(now, domain.ReportDaily, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	fixture := reportEvidenceFixture{}
	tx, err := runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'fixture','Report Evidence Editor','editor') RETURNING id`, fmt.Sprintf("report-evidence-%d@example.test", now.UnixNano())).Scan(&fixture.actorID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss',$1,$2) RETURNING id`, fmt.Sprintf("report-source-%d", now.UnixNano()), "https://report.example.test/feed").Scan(&fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	var documentID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO documents (source_connection_id,document_key,document_state)
VALUES ($1,$2,'active') RETURNING id`, fixture.sourceID, strings.Repeat("a", 64)).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO document_versions (
document_id,source_observation_id,revision_no,version_key,body_origin,completeness,word_count,language,
content_sha256,extractor_version,extractor_profile_version,extractor_profile_sha256,lifecycle_state,captured_at)
VALUES ($1,900001,1,$2,'feed_content','full',16,'zh-CN',$3,'fixture-v1','fixture-profile-v1',$4,'readable',$5)
RETURNING id`, documentID, strings.Repeat("b", 64), strings.Repeat("d", 64), strings.Repeat("e", 64), now).Scan(&fixture.documentVersionID); err != nil {
		t.Fatal(err)
	}
	insertDecision := func(action string, policyOffset int64, retentionDays any) int64 {
		var decisionID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from,retention_days)
VALUES ($1,$2,$3,1,'source_endpoint',$4,200,'report fixture','document_version',$5,$6,$7,'allow',ARRAY['fixture'],'fixture',$8,$8,$9)
RETURNING id`, 910000+policyOffset, fixture.sourceID, 920000+policyOffset, fmt.Sprint(fixture.sourceID),
			fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64), action, now.Add(-time.Hour), retentionDays).Scan(&decisionID); err != nil {
			t.Fatal(err)
		}
		return decisionID
	}
	fixture.quoteDecisionID = insertDecision("quote", fixture.documentVersionID*2, nil)
	fixture.retainDecisionID = insertDecision("retain", fixture.documentVersionID*2+1, 30)
	if err := tx.QueryRowContext(ctx, `INSERT INTO document_text_quote_selectors (
source_connection_id,document_version_id,plaintext_artifact_id,markdown_artifact_id,quote_rights_decision_id,retain_rights_decision_id,
exact_quote,prefix,suffix,utf8_byte_start,utf8_byte_end,quote_sha256,plaintext_sha256,normalization_version,selector_version,
anchor_map_sha256,retention_until,created_at)
VALUES ($1,$2,930001,930002,$3,$4,'主体完成发布','今日','',6,24,$5,$6,
'nfc-lf-collapse-space-v1','w3c-text-quote-position-nfc-utf8-v1',$7,$8,$9) RETURNING id`,
		fixture.sourceID, fixture.documentVersionID, fixture.quoteDecisionID, fixture.retainDecisionID,
		strings.Repeat("c", 64), strings.Repeat("d", 64), strings.Repeat("f", 64), now.Add(48*time.Hour), now.Add(-time.Hour)).Scan(&fixture.selectorID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO micro_events (
event_key,status,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version)
VALUES ($1,'active','主体','发布',$2,'fixture-clustering-v1') RETURNING id`, strings.Repeat("1", 64), now.Add(-time.Hour)).Scan(&fixture.microEventID); err != nil {
		t.Fatal(err)
	}
	var claimID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO claims (
micro_event_id,micro_event_version,claim_hash,subject,predicate,object)
VALUES ($1,1,$2,'主体','完成','发布') RETURNING id`, fixture.microEventID, strings.Repeat("2", 64)).Scan(&claimID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO claim_evidence_versions (
claim_id,document_version_id,text_quote_selector_id,content_family_id,lineage_root_document_version_id,relation,
quote_sha256,plaintext_sha256,selector_version,captured_at_snapshot,extraction_schema_version,decision_origin,actor_user_id,
idempotency_key,command_fingerprint,retention_until)
VALUES ($1,$2,$3,940001,$2,'asserts',$4,$5,'w3c-text-quote-position-nfc-utf8-v1',$6,
'fixture-claim-v1','manual',$7,$8,$9,$10) RETURNING id`, claimID, fixture.documentVersionID, fixture.selectorID,
		strings.Repeat("c", 64), strings.Repeat("d", 64), period.Start.Add(time.Hour), fixture.actorID,
		fmt.Sprintf("report-evidence-%d", now.UnixNano()), strings.Repeat("3", 64), now.Add(48*time.Hour)).Scan(&fixture.claimEvidenceVersionID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO micro_event_summaries (
micro_event_id,micro_event_version,summary_profile_version,idempotency_key,command_fingerprint)
VALUES ($1,1,'fixture-summary-v1',$2,$3) RETURNING id`, fixture.microEventID,
		fmt.Sprintf("report-summary-%d", now.UnixNano()), strings.Repeat("4", 64)).Scan(&fixture.summaryID); err != nil {
		t.Fatal(err)
	}
	var sentenceID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO micro_event_summary_sentences (
micro_event_summary_id,ordinal,sentence,editorial_note,decision_origin,actor_user_id)
VALUES ($1,0,'主体完成发布',false,'manual',$2) RETURNING id`, fixture.summaryID, fixture.actorID).Scan(&sentenceID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO micro_event_summary_sentence_evidences
(summary_sentence_id,claim_evidence_version_id,ordinal) VALUES ($1,$2,0)`, sentenceID, fixture.claimEvidenceVersionID); err != nil {
		t.Fatal(err)
	}
	var evidenceProfileID, evidenceSnapshotID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO evidence_state_profiles
(algorithm_version,status,activated_by_user_id,activated_at) VALUES ($1,'active',$2,$3) RETURNING id`,
		fmt.Sprintf("fixture-evidence-%d", now.UnixNano()), fixture.actorID, now.Add(-time.Hour)).Scan(&evidenceProfileID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO evidence_state_snapshots (
micro_event_id,micro_event_version,evidence_state_profile_id,algorithm_version,evidence_set_hash,evidence_state,
independent_origin_count,reason_codes,calculated_at)
VALUES ($1,1,$2,$3,$4,'single_origin',1,'["fixture"]',$5) RETURNING id`, fixture.microEventID,
		evidenceProfileID, fmt.Sprintf("fixture-evidence-%d", now.UnixNano()), strings.Repeat("5", 64), now).Scan(&evidenceSnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO evidence_state_snapshot_items
(evidence_state_snapshot_id,claim_evidence_version_id,ordinal) VALUES ($1,$2,0)`,
		evidenceSnapshotID, fixture.claimEvidenceVersionID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO micro_event_updates (
micro_event_id,micro_event_version,window_ended_at,window_profile,heat_profile_id,heat_profile_version,
evidence_state_profile_id,evidence_state_algorithm_version,heat_snapshot_1h_id,heat_snapshot_6h_id,heat_snapshot_24h_id,
evidence_state_snapshot_id,heat_score,evidence_state,independent_origin_count,reason_codes,refresh_key)
VALUES ($1,1,$2,'1h-6h-24h-v1',950001,'fixture-heat-v1',$3,$4,950002,950003,950004,$5,
77,'single_origin',1,'["fixture"]',$6) RETURNING id`, fixture.microEventID, now, evidenceProfileID,
		fmt.Sprintf("fixture-evidence-%d", now.UnixNano()), evidenceSnapshotID, strings.Repeat("6", 64)).Scan(&fixture.microEventUpdateID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	actorID := fixture.actorID
	fixture.report = domain.Report{ID: 7001, Version: 1, VersionNo: 1, Type: domain.ReportDaily, Period: period,
		Title: "日报", Status: domain.ReportDraft, Items: []domain.Item{{MicroEventID: fixture.microEventID,
			MicroEventVersion: 1, MicroEventUpdateID: fixture.microEventUpdateID, MicroEventSummaryID: fixture.summaryID,
			Rank: 1, Title: "主体 发布", Summary: "主体完成发布", InclusionReason: "period_latest_product_event_update",
			HeatScore: 77, EvidenceSetHash: strings.Repeat("5", 64), ReasonCodes: []string{"fixture"},
			Sentences: []domain.Sentence{{SourceSummarySentenceID: sentenceID, Ordinal: 0, Text: "主体完成发布",
				DecisionOrigin: "manual", ActorUserID: &actorID, ClaimEvidenceVersionIDs: []int64{fixture.claimEvidenceVersionID}}}}}}
	fixture.report.InputSnapshotHash = domain.ComputeInputSnapshotHash(fixture.report)
	return fixture
}

func setClaimEvidenceCapturedAt(t *testing.T, runtime *database.Runtime, evidenceID int64, capturedAt time.Time) {
	t.Helper()
	tx, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE claim_evidence_versions SET captured_at_snapshot=$2 WHERE id=$1`, evidenceID, capturedAt); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func insertRightsDeny(t *testing.T, runtime *database.Runtime, fixture reportEvidenceFixture) {
	t.Helper()
	tx, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO source_rights_decisions (
decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,priority_rank,
basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,evaluator,evaluated_at,effective_from)
VALUES (990001,$1,990002,1,'observation','report-revoke',400,'report revoke','document_version',$2,$3,
'quote','deny',ARRAY['revoked'],'fixture',now(),now())`, fixture.sourceID, fmt.Sprint(fixture.documentVersionID), strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
