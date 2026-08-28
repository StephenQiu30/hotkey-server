//go:build integration

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	reportapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	reportdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	reportpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestReportApprovalCreatesKnowledgeProposalInThePublicationTransaction(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	var editorID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,display_name,role,status) VALUES ('archive-editor@example.test','fixture','Archive Editor','editor','active') RETURNING id`).Scan(&editorID); err != nil {
		t.Fatal(err)
	}
	reports := reportpostgres.NewRepository(runtime)
	knowledge := knowledgepostgres.NewRepository(runtime)
	proposals := knowledgeapplication.NewProposalService(knowledge, knowledge)
	service, err := reportapplication.NewService(reports)
	if err != nil {
		t.Fatal(err)
	}
	service.SetArchivePlanner(buildReportArchivePlanner(knowledge, proposals))

	created := createArchiveIntegrationReport(t, ctx, reports, editorID, time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC))
	subject := identitydomain.Subject{UserID: editorID, SessionID: 71, Role: identitydomain.RoleEditor}
	pending, err := service.SubmitForApproval(ctx, reportapplication.RevisionLifecycleInput{Subject: subject, ReportID: created.ID, ExpectedVersion: created.Version})
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.ApproveRevision(ctx, reportapplication.RevisionLifecycleInput{Subject: subject, ReportID: pending.ID, ExpectedVersion: pending.Version})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != reportdomain.ReportPublished || !published.Frozen {
		t.Fatalf("published report = %#v", published)
	}

	var path, documentStatus, proposalStatus, body string
	if err := runtime.SQL.QueryRowContext(ctx, `
SELECT document.vault_path,document.status,proposal.status,proposal.proposed_body
FROM knowledge_documents AS document
JOIN knowledge_change_proposals AS proposal ON proposal.document_id=document.id
WHERE document.report_id=$1`, published.ID).Scan(&path, &documentStatus, &proposalStatus, &body); err != nil {
		t.Fatal(err)
	}
	if path != "reports/report-1.md" || documentStatus != "planned" || proposalStatus != "pending" || body == "" {
		t.Fatalf("archive projection = path %q document %q proposal %q body %q", path, documentStatus, proposalStatus, body)
	}
}

func TestReportApprovalRollsBackKnowledgeDocumentWhenProposalInsertFails(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var editorID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,display_name,role,status) VALUES ('archive-rollback@example.test','fixture','Archive Rollback','editor','active') RETURNING id`).Scan(&editorID); err != nil {
		t.Fatal(err)
	}
	reports := reportpostgres.NewRepository(runtime)
	knowledge := knowledgepostgres.NewRepository(runtime)
	service, err := reportapplication.NewService(reports)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("proposal insert unavailable")
	service.SetArchivePlanner(buildReportArchivePlanner(knowledge, &reportArchiveProposalCreatorFake{err: want}))
	created := createArchiveIntegrationReport(t, ctx, reports, editorID, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	subject := identitydomain.Subject{UserID: editorID, SessionID: 72, Role: identitydomain.RoleEditor}
	pending, err := service.SubmitForApproval(ctx, reportapplication.RevisionLifecycleInput{Subject: subject, ReportID: created.ID, ExpectedVersion: created.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveRevision(ctx, reportapplication.RevisionLifecycleInput{Subject: subject, ReportID: pending.ID, ExpectedVersion: pending.Version}); !errors.Is(err, want) {
		t.Fatalf("ApproveRevision() error = %v, want %v", err, want)
	}

	stored, err := reports.Get(ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != reportdomain.ReportPendingApproval || stored.Version != pending.Version {
		t.Fatalf("report escaped transaction = %#v", stored)
	}
	var documents int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_documents WHERE report_id=$1`, pending.ID).Scan(&documents); err != nil {
		t.Fatal(err)
	}
	if documents != 0 {
		t.Fatalf("knowledge documents after rollback = %d, want 0", documents)
	}
}

func createArchiveIntegrationReport(t *testing.T, ctx context.Context, repository *reportpostgres.Repository, actorID int64, at time.Time) reportdomain.Report {
	t.Helper()
	report, err := reportapplication.NewBuilder().Build(1, reportdomain.ReportDaily, at, time.UTC, nil)
	if err != nil {
		t.Fatal(err)
	}
	report.Summary = "No events matched the requested period."
	report.CreatedBy, report.UpdatedBy = &actorID, &actorID
	report.InputSnapshotHash = reportdomain.ComputeInputSnapshotHash(report)
	created, err := repository.Create(ctx, report)
	if err != nil {
		t.Fatal(err)
	}
	if computed := reportdomain.ComputeInputSnapshotHash(created); created.InputSnapshotHash != computed {
		t.Fatalf("created report snapshot hash = %q, recomputed %q (before %#v, after %#v)", created.InputSnapshotHash, computed, report, created)
	}
	return created
}
