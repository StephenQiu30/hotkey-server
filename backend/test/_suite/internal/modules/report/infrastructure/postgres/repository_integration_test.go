//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	reportapp "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

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

	if _, err := builder.Publish(report); !errors.Is(err, domain.ErrEvidenceInvalid) {
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
