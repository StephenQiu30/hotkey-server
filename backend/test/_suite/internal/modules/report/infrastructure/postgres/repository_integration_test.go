//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	reportapp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestReportListCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	codec, err := pagination.NewCodec("report-list-cursor-secret-for-tests-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositoryWithCursorCodec(runtime, codec)
	base := time.Date(2026, time.August, 29, 0, 0, 0, 0, time.UTC)
	initial := make(map[int64]struct{}, 4)
	for index := 0; index < 4; index++ {
		initial[insertCursorReport(t, runtime, base.AddDate(0, 0, index), domain.ReportDaily)] = struct{}{}
	}

	first, err := repository.List(ctx, domain.ListQuery{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v/%v", first, err)
	}
	if first.NextCursor == fmt.Sprintf("%d", first.Items[1].ID) || !strings.Contains(first.NextCursor, ".") {
		t.Fatalf("cursor is not opaque and signed: %q", first.NextCursor)
	}

	tampered := "A" + first.NextCursor[1:]
	if tampered == first.NextCursor {
		tampered = "B" + first.NextCursor[1:]
	}
	if _, err := repository.List(ctx, domain.ListQuery{Limit: 2, Cursor: tampered}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("tampered cursor error = %v, want invalid input", err)
	}
	daily := domain.ReportDaily
	if _, err := repository.List(ctx, domain.ListQuery{Limit: 2, Cursor: first.NextCursor, Type: &daily}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-filter cursor error = %v, want invalid input", err)
	}

	concurrentID := insertCursorReport(t, runtime, base.AddDate(0, 0, 10), domain.ReportWeekly)
	second, err := repository.List(ctx, domain.ListQuery{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 2 || second.NextCursor != "" {
		t.Fatalf("second page = %#v/%v", second, err)
	}
	seen := make(map[int64]struct{}, 4)
	for _, report := range append(first.Items, second.Items...) {
		if _, duplicate := seen[report.ID]; duplicate {
			t.Fatalf("report %d repeated across pages", report.ID)
		}
		if report.ID == concurrentID {
			t.Fatalf("concurrent report %d leaked into an existing traversal", concurrentID)
		}
		if _, expected := initial[report.ID]; !expected {
			t.Fatalf("unexpected report %d in traversal", report.ID)
		}
		seen[report.ID] = struct{}{}
	}
	if len(seen) != len(initial) {
		t.Fatalf("traversal returned %d initial reports, want %d", len(seen), len(initial))
	}

	expiringCodec, err := pagination.NewCodec("expiring-report-cursor-secret-for-tests-32b", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	expiring := NewRepositoryWithCursorCodec(runtime, expiringCodec)
	expiringFirst, err := expiring.List(ctx, domain.ListQuery{Limit: 1})
	if err != nil || expiringFirst.NextCursor == "" {
		t.Fatalf("expiring first page = %#v/%v", expiringFirst, err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := expiring.List(ctx, domain.ListQuery{Limit: 1, Cursor: expiringFirst.NextCursor}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired cursor error = %v, want invalid input", err)
	}
}

func insertCursorReport(t *testing.T, runtime *database.Runtime, periodStart time.Time, reportType domain.ReportType) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO reports
(report_type,period_start,period_end,timezone,title,input_snapshot_hash,status,version_no)
VALUES ($1,$2,$3,'UTC',$4,repeat('a',64),'draft',1) RETURNING id`, reportType, periodStart, periodStart.Add(24*time.Hour),
		fmt.Sprintf("cursor-report-%d", periodStart.Unix())).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestReportRepositoryKeepsLegacyItemsReadableButRejectsNewPublication(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	var eventID, eventUpdateID int64
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('report-fixture-' || md5(random()::text), 'Report fixture', 'fixture', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO event_updates (event_id, sequence_no, kind, summary, observed_at, reason_codes, before_state, after_state, evidence_set_hash, idempotency_key) VALUES ($1,1,'event_created','Snapshot',$2,ARRAY['first_snapshot'],'{}','{"heat_score":77}',repeat('a',64),repeat('b',64)) RETURNING id`, eventID, now).Scan(&eventUpdateID); err != nil {
		t.Fatal(err)
	}

	builder := reportapp.NewBuilder()
	report, err := builder.Build(7001, domain.ReportDaily, now, time.UTC, []reportapp.EventSnapshot{{EventID: eventID, EventUpdateID: eventUpdateID, Title: "Report event", Summary: "Snapshot", HeatScore: 77, EvidenceSetHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReasonCodes: []string{"first_snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	if err := repository.Save(ctx, report); err != nil {
		t.Fatal(err)
	}
	var items int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM report_items WHERE report_id = $1`, report.ID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items != 1 {
		t.Fatalf("report items = %d, want 1", items)
	}

	if err := report.ValidatePublicationShape(); !errors.Is(err, domain.ErrEvidenceInvalid) {
		t.Fatalf("publish legacy report error = %v, want ErrEvidenceInvalid", err)
	}
	loaded, err := repository.Get(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.ReportDraft || loaded.Frozen || len(loaded.Items) != 1 || loaded.PublishedAt != nil {
		t.Fatalf("loaded report = %#v", loaded)
	}
	if len(loaded.Items[0].ReasonCodes) != 1 || loaded.Items[0].ReasonCodes[0] != "first_snapshot" {
		t.Fatalf("loaded report reason codes = %#v", loaded.Items[0].ReasonCodes)
	}
	page, err := repository.List(ctx, domain.ListQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != report.ID || len(page.Items[0].Items) != 1 {
		t.Fatalf("list reports = %#v/%v", page, err)
	}
}

func TestCandidateReaderSelectsLatestUpdateInsidePeriod(t *testing.T) {
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
	items, err := NewCandidateReader(runtime).ListForPeriod(ctx, nil, fixture.report.Period.Start, fixture.report.Period.End, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].MicroEventID != fixture.microEventID ||
		items[0].MicroEventUpdateID != fixture.microEventUpdateID || items[0].MicroEventSummaryID != fixture.summaryID ||
		len(items[0].Sentences) != 1 || len(items[0].Sentences[0].ClaimEvidenceVersionIDs) != 1 ||
		items[0].Sentences[0].ClaimEvidenceVersionIDs[0] != fixture.claimEvidenceVersionID {
		t.Fatalf("period candidates = %#v", items)
	}
}
