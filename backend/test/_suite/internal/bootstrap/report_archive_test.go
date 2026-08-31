package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	reportdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type reportArchiveDocumentStoreFake struct {
	reportID  int64
	vaultPath string
	actorID   *int64
	document  knowledgedomain.Document
	err       error
}

func (store *reportArchiveDocumentStoreFake) EnsureReportDocument(_ context.Context, reportID int64, vaultPath string, actorID *int64) (knowledgedomain.Document, error) {
	store.reportID, store.vaultPath, store.actorID = reportID, vaultPath, actorID
	return store.document, store.err
}

type reportArchiveProposalCreatorFake struct {
	documentID, baseRevision int64
	baseHash                 string
	frontmatter              string
	body                     string
	reason                   string
	err                      error
}

func (creator *reportArchiveProposalCreatorFake) Create(_ context.Context, documentID, baseRevision int64, baseHash, frontmatter, body, reason string) (knowledgedomain.Proposal, error) {
	creator.documentID, creator.baseRevision, creator.baseHash = documentID, baseRevision, baseHash
	creator.frontmatter, creator.body, creator.reason = frontmatter, body, reason
	return knowledgedomain.Proposal{ID: 1, Version: 1, DocumentID: documentID, BaseRevisionNo: baseRevision, BaseHash: baseHash, ProposedFrontmatter: frontmatter, ProposedBody: body, Reason: reason, Status: knowledgedomain.ProposalPending}, creator.err
}

func TestReportArchivePlannerCreatesStableReviewableKnowledgeProposal(t *testing.T) {
	reviewerID := int64(29)
	report := publishedReportForArchiveTest(reviewerID)
	document := knowledgedomain.Document{
		ID: 41, Version: 1, RevisionNo: 0, Type: knowledgedomain.DocumentReport,
		VaultPath: "reports/report-17.md", ContentHash: knowledgedomain.HashContent("", ""), GeneratedHash: knowledgedomain.HashContent("", ""),
		Status: knowledgedomain.DocumentPlanned, ReportID: &report.ID,
	}
	documents := &reportArchiveDocumentStoreFake{document: document}
	proposals := &reportArchiveProposalCreatorFake{}
	planner := buildReportArchivePlanner(documents, proposals)

	if err := planner.Prepare(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if documents.reportID != report.ID || documents.vaultPath != "reports/report-17.md" || documents.actorID == nil || *documents.actorID != reviewerID {
		t.Fatalf("knowledge document input = report %d path %q actor %#v", documents.reportID, documents.vaultPath, documents.actorID)
	}
	if proposals.documentID != document.ID || proposals.baseRevision != 0 || proposals.baseHash != document.ContentHash {
		t.Fatalf("proposal base = document %d revision %d hash %q", proposals.documentID, proposals.baseRevision, proposals.baseHash)
	}
	if proposals.frontmatter != `{"title":"日报 #17"}` || proposals.reason != "report_published" {
		t.Fatalf("proposal metadata = %q / %q", proposals.frontmatter, proposals.reason)
	}
	for _, fragment := range []string{"# 日报知识投影", "报告 ID: `17`", "报告版本: `1`", "No events matched the requested period.", "本周期暂无事件。"} {
		if !strings.Contains(proposals.body, fragment) {
			t.Errorf("proposal body does not contain %q: %s", fragment, proposals.body)
		}
	}
}

func TestReportArchivePlannerRejectsMutableOrUnsafeReportsBeforeWriting(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reportdomain.Report)
	}{
		{name: "pending", mutate: func(report *reportdomain.Report) {
			report.Status, report.Frozen = reportdomain.ReportPendingApproval, false
		}},
		{name: "marker injection", mutate: func(report *reportdomain.Report) { report.Summary = knowledgedomain.AutomaticRegionBegin }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := publishedReportForArchiveTest(29)
			test.mutate(&report)
			report.InputSnapshotHash = reportdomain.ComputeInputSnapshotHash(report)
			documents := &reportArchiveDocumentStoreFake{}
			proposals := &reportArchiveProposalCreatorFake{}
			planner := buildReportArchivePlanner(documents, proposals)
			if err := planner.Prepare(context.Background(), report); !errors.Is(err, sharedrepository.ErrInvalidInput) {
				t.Fatalf("Prepare() error = %v, want ErrInvalidInput", err)
			}
			if documents.reportID != 0 || proposals.documentID != 0 {
				t.Fatalf("invalid report escaped to stores: %#v / %#v", documents, proposals)
			}
		})
	}
}

func TestReportArchivePlannerPropagatesProposalFailure(t *testing.T) {
	report := publishedReportForArchiveTest(29)
	document := knowledgedomain.Document{ID: 41, Version: 1, RevisionNo: 0, Type: knowledgedomain.DocumentReport, VaultPath: "reports/report-17.md", ContentHash: knowledgedomain.HashContent("", ""), GeneratedHash: knowledgedomain.HashContent("", ""), Status: knowledgedomain.DocumentPlanned, ReportID: &report.ID}
	want := errors.New("proposal unavailable")
	planner := buildReportArchivePlanner(&reportArchiveDocumentStoreFake{document: document}, &reportArchiveProposalCreatorFake{err: want})
	if err := planner.Prepare(context.Background(), report); !errors.Is(err, want) {
		t.Fatalf("Prepare() error = %v, want %v", err, want)
	}
}

func publishedReportForArchiveTest(reviewerID int64) reportdomain.Report {
	period, _ := reportdomain.PeriodFor(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), reportdomain.ReportDaily, time.UTC)
	publishedAt := time.Date(2026, 8, 28, 12, 30, 0, 0, time.UTC)
	report := reportdomain.Report{
		ID: 17, Version: 3, VersionNo: 1, Type: reportdomain.ReportDaily, Period: period,
		Title: "日报", Summary: "No events matched the requested period.", Status: reportdomain.ReportPublished,
		Frozen: true, PublishedAt: &publishedAt, ReviewedAt: &publishedAt, ReviewedBy: &reviewerID,
	}
	report.InputSnapshotHash = reportdomain.ComputeInputSnapshotHash(report)
	return report
}
