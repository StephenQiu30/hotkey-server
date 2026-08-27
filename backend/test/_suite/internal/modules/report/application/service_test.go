package application

import (
	"context"
	"errors"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type serviceStoreFake struct {
	reports                map[int64]domain.Report
	publicationError       error
	publicationValidations int
	nextID                 int64
}

func (fake *serviceStoreFake) Transition(_ context.Context, transition domain.RevisionTransition) (domain.Report, error) {
	report, ok := fake.reports[transition.ReportID]
	if !ok {
		return domain.Report{}, sharedrepository.ErrNotFound
	}
	if report.Version != transition.ExpectedVersion || report.Status != transition.From {
		return domain.Report{}, sharedrepository.ErrConflict
	}
	report.Version++
	report.Status = transition.To
	report.Frozen = transition.To == domain.ReportPublished
	report.UpdatedBy = &transition.ActorID
	fake.reports[report.ID] = report
	return report, nil
}

func (fake *serviceStoreFake) Save(_ context.Context, report domain.Report) error {
	fake.reports[report.ID] = report
	return nil
}

func (fake *serviceStoreFake) Get(_ context.Context, reportID int64) (domain.Report, error) {
	report, ok := fake.reports[reportID]
	if !ok {
		return domain.Report{}, sharedrepository.ErrNotFound
	}
	return report, nil
}

func (fake *serviceStoreFake) List(_ context.Context, _ domain.ListQuery) (domain.Page, error) {
	return domain.Page{}, nil
}

func (fake *serviceStoreFake) FindByPeriod(_ context.Context, reportType domain.ReportType, monitorID *int64, start, end time.Time) (domain.Report, error) {
	var latest domain.Report
	for _, report := range fake.reports {
		if report.Type == reportType && sameOptionalInt64(report.MonitorID, monitorID) && report.Period.Start.Equal(start) && report.Period.End.Equal(end) && report.VersionNo > latest.VersionNo {
			latest = report
		}
	}
	if latest.ID == 0 {
		return domain.Report{}, sharedrepository.ErrNotFound
	}
	return latest, nil
}

func (fake *serviceStoreFake) Create(_ context.Context, report domain.Report) (domain.Report, error) {
	if fake.nextID <= 0 {
		fake.nextID = 100
	}
	report.ID = fake.nextID
	fake.nextID++
	fake.reports[report.ID] = report
	return report, nil
}

func sameOptionalInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

type reportSnapshotReaderFake struct{ events []EventSnapshot }

func (fake reportSnapshotReaderFake) ListForPeriod(context.Context, *int64, time.Time, time.Time, int) ([]EventSnapshot, error) {
	return fake.events, nil
}

func (fake *serviceStoreFake) ValidatePublication(_ context.Context, _ domain.Report) error {
	fake.publicationValidations++
	return fake.publicationError
}

func (fake *serviceStoreFake) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	before := make(map[int64]domain.Report, len(fake.reports))
	for id, report := range fake.reports {
		before[id] = report
	}
	if err := fn(ctx); err != nil {
		fake.reports = before
		return err
	}
	return nil
}

type failingArchive struct{ err error }

func (archive failingArchive) Prepare(context.Context, domain.Report) error { return archive.err }

func TestServiceApprovalFreezesPendingRevisionAndRejectsRepeat(t *testing.T) {
	period, err := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	draft := citedReportDraft(period, 7)
	store := &serviceStoreFake{reports: map[int64]domain.Report{7: draft}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	analyst := identitydomain.Subject{UserID: 3, SessionID: 30, Role: identitydomain.RoleAnalyst}
	editor := identitydomain.Subject{UserID: 4, SessionID: 40, Role: identitydomain.RoleEditor}
	pending, err := service.SubmitForApproval(context.Background(), RevisionLifecycleInput{Subject: analyst, ReportID: 7, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	published, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: 7, ExpectedVersion: pending.Version})
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != domain.ReportPublished || !published.Frozen || published.Version != 3 {
		t.Fatalf("published report = %#v", published)
	}
	if store.publicationValidations != 2 {
		t.Fatalf("publication validations = %d, want 2", store.publicationValidations)
	}
	if _, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: 7, ExpectedVersion: published.Version}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("repeat approval error = %v, want ErrConflict", err)
	}
}

func TestServiceBuildUsesTimezoneAndDeterministicFallback(t *testing.T) {
	store := &serviceStoreFake{reports: make(map[int64]domain.Report)}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Build(context.Background(), BuildInput{ID: 8, Type: domain.ReportWeekly, At: time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC), Timezone: "Asia/Shanghai", Events: []EventSnapshot{{EventID: 2, EventUpdateID: 12, Title: "event", Summary: "snapshot", HeatScore: 91, EvidenceSetHash: testHash, ReasonCodes: []string{"rising"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != domain.ReportDraft || report.Title != "周报" || report.Summary == "" || report.Period.Location.String() != "Asia/Shanghai" || report.Items[0].InclusionReason == "" {
		t.Fatalf("built report = %#v", report)
	}
}

func TestServiceApprovalRollsBackWhenArchiveProposalFails(t *testing.T) {
	period, _ := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	draft := citedReportDraft(period, 11)
	draft.Status, draft.Version = domain.ReportPendingApproval, 2
	store := &serviceStoreFake{reports: map[int64]domain.Report{draft.ID: draft}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	service.SetArchivePlanner(failingArchive{err: errors.New("archive unavailable")})
	editor := identitydomain.Subject{UserID: 4, SessionID: 40, Role: identitydomain.RoleEditor}
	if _, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: draft.ID, ExpectedVersion: draft.Version}); err == nil {
		t.Fatal("ApproveRevision() error = nil")
	}
	if stored := store.reports[draft.ID]; stored.Status != domain.ReportPendingApproval || stored.Version != 2 {
		t.Fatalf("report escaped failed transaction: %#v", stored)
	}
}

func TestServiceApprovalRejectsInvalidEvidenceBeforeAnyWrite(t *testing.T) {
	period, _ := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	draft := citedReportDraft(period, 13)
	draft.Status, draft.Version = domain.ReportPendingApproval, 2
	store := &serviceStoreFake{reports: map[int64]domain.Report{draft.ID: draft}, publicationError: domain.ErrEvidenceInvalid}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	editor := identitydomain.Subject{UserID: 7, SessionID: 70, Role: identitydomain.RoleEditor}
	if _, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: draft.ID, ExpectedVersion: draft.Version}); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("ApproveRevision() error = %v, want ErrEvidenceInvalid", err)
	}
	if stored := store.reports[draft.ID]; stored.Status != domain.ReportPendingApproval || stored.Version != 2 || stored.UpdatedBy != nil {
		t.Fatalf("invalid evidence changed report: %#v", stored)
	}
}

func TestServiceRequiresPendingRevisionAndExpectedVersionForApproval(t *testing.T) {
	period, _ := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	draft := citedReportDraft(period, 17)
	store := &serviceStoreFake{reports: map[int64]domain.Report{draft.ID: draft}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	analyst := identitydomain.Subject{UserID: 3, SessionID: 30, Role: identitydomain.RoleAnalyst}
	editor := identitydomain.Subject{UserID: 4, SessionID: 40, Role: identitydomain.RoleEditor}

	pending, err := service.SubmitForApproval(context.Background(), RevisionLifecycleInput{Subject: analyst, ReportID: draft.ID, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != domain.ReportPendingApproval || pending.Version != 2 || pending.Frozen {
		t.Fatalf("pending revision = %#v", pending)
	}
	if _, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: draft.ID, ExpectedVersion: 1}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale approval error = %v, want ErrConflict", err)
	}
	approved, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: draft.ID, ExpectedVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.ReportPublished || approved.Version != 3 || !approved.Frozen || approved.UpdatedBy == nil || *approved.UpdatedBy != editor.UserID {
		t.Fatalf("approved revision = %#v", approved)
	}
}

func TestServiceRejectsAnalystApprovalAndRecordsRejectedRevision(t *testing.T) {
	period, _ := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	draft := citedReportDraft(period, 19)
	draft.Status, draft.Version = domain.ReportPendingApproval, 2
	store := &serviceStoreFake{reports: map[int64]domain.Report{draft.ID: draft}}
	service, _ := NewService(store)
	analyst := identitydomain.Subject{UserID: 3, SessionID: 30, Role: identitydomain.RoleAnalyst}
	editor := identitydomain.Subject{UserID: 4, SessionID: 40, Role: identitydomain.RoleEditor}
	if _, err := service.ApproveRevision(context.Background(), RevisionLifecycleInput{Subject: analyst, ReportID: draft.ID, ExpectedVersion: 2}); appErrorCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("analyst approval error = %v", err)
	}
	rejected, err := service.RejectRevision(context.Background(), RevisionLifecycleInput{Subject: editor, ReportID: draft.ID, ExpectedVersion: 2, ReasonCode: "insufficient_context"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != domain.ReportRejected || rejected.Version != 3 || rejected.Frozen {
		t.Fatalf("rejected revision = %#v", rejected)
	}
}

func TestServiceRegenerationCreatesNextDraftWithoutOverwritingApprovedRevision(t *testing.T) {
	at := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	period, _ := domain.PeriodFor(at, domain.ReportDaily, time.UTC)
	approved := citedReportDraft(period, 23)
	approved.Status, approved.Frozen, approved.Version = domain.ReportPublished, true, 3
	store := &serviceStoreFake{reports: map[int64]domain.Report{approved.ID: approved}, nextID: 24}
	reader := reportSnapshotReaderFake{events: []EventSnapshot{{MicroEventID: 9, MicroEventVersion: 2, MicroEventUpdateID: 19,
		MicroEventSummaryID: 29, Title: "event", Summary: "summary", HeatScore: 80, EvidenceSetHash: testHash,
		ReasonCodes: []string{"rising"}, Sentences: approved.Items[0].Sentences}}}
	service, err := NewService(store, reader)
	if err != nil {
		t.Fatal(err)
	}
	analyst := identitydomain.Subject{UserID: 3, SessionID: 30, Role: identitydomain.RoleAnalyst}
	regenerated, err := service.CreateDraft(context.Background(), CreateInput{Subject: analyst, Type: domain.ReportDaily, At: at, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if regenerated.ID == approved.ID || regenerated.VersionNo != 2 || regenerated.Status != domain.ReportDraft {
		t.Fatalf("regenerated report = %#v", regenerated)
	}
	if original := store.reports[approved.ID]; original.Status != domain.ReportPublished || !original.Frozen || original.Version != 3 || original.Title != approved.Title {
		t.Fatalf("approved revision overwritten = %#v", original)
	}
}

func appErrorCode(err error) int {
	var appErr *sharederrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return 0
}

func citedReportDraft(period domain.Period, id int64) domain.Report {
	actorID := int64(3)
	report := domain.Report{ID: id, Version: 1, VersionNo: 1, Type: domain.ReportDaily, Period: period, Title: "daily", Status: domain.ReportDraft,
		Items: []domain.Item{{MicroEventID: 9, MicroEventVersion: 2, MicroEventUpdateID: 19, MicroEventSummaryID: 29,
			Rank: 1, Title: "event", HeatScore: 80, EvidenceSetHash: testHash, ReasonCodes: []string{"rising"},
			Sentences: []domain.Sentence{{SourceSummarySentenceID: 39, Ordinal: 0, Text: "Sourced fact.",
				DecisionOrigin: "manual", ActorUserID: &actorID, ClaimEvidenceVersionIDs: []int64{49}}}}}}
	report.InputSnapshotHash = domain.ComputeInputSnapshotHash(report)
	return report
}

const testHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
