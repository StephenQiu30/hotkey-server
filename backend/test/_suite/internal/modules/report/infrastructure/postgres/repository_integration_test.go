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
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestReportRepositoryPersistsItemsAndFreezesPublishedVersion(t *testing.T) {
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

	published, err := builder.Publish(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, published); err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(ctx, published); !errors.Is(err, sharedrepository.ErrImmutable) {
		t.Fatalf("save published report error = %v, want ErrImmutable", err)
	}
	if err := repository.Save(ctx, report); !errors.Is(err, sharedrepository.ErrImmutable) {
		t.Fatalf("save stale draft error = %v, want ErrImmutable", err)
	}
	loaded, err := repository.Get(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.ReportPublished || !loaded.Frozen || len(loaded.Items) != 1 || loaded.PublishedAt == nil {
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
	start := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	var eventID, firstID, latestID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at) VALUES ('period-event','周期事件','projection','active',$1,$2) RETURNING id`, start.Add(-time.Hour), start.Add(time.Hour)).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	insert := func(sequence int, observed time.Time, heat int, hashChar string, id *int64) {
		err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO event_updates (event_id,sequence_no,kind,summary,observed_at,reason_codes,before_state,after_state,evidence_set_hash,idempotency_key) VALUES ($1,$2,'metric_changed',$3,$4,ARRAY['heat_delta'],'{}',jsonb_build_object('heat_score',$5::numeric),repeat($6::text,64),md5(random()::text)||md5(random()::text)) RETURNING id`, eventID, sequence, fmt.Sprintf("snapshot-%d", sequence), observed, heat, hashChar).Scan(id)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(1, start.Add(-time.Minute), 20, "a", &firstID)
	insert(2, start.Add(time.Hour), 60, "b", &firstID)
	insert(3, start.Add(2*time.Hour), 88, "c", &latestID)
	items, err := NewCandidateReader(runtime).ListForPeriod(ctx, nil, start, start.AddDate(0, 0, 1), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EventUpdateID != latestID || items[0].HeatScore != 88 || items[0].EvidenceSetHash != strings.Repeat("c", 64) {
		t.Fatalf("period candidates = %#v", items)
	}
}
