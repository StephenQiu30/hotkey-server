//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	reportapp "github.com/StephenQiu30/hotkey-server/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
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

	var eventID int64
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('report-fixture-' || md5(random()::text), 'Report fixture', 'fixture', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}

	builder := reportapp.NewBuilder()
	report, err := builder.Build(7001, domain.ReportDaily, now, time.UTC, []reportapp.EventSnapshot{{EventID: eventID, Title: "Report event", Summary: "Snapshot", HeatScore: 77}})
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
	if err := repository.Save(ctx, report); !errors.Is(err, sharedrepository.ErrImmutable) {
		t.Fatalf("save stale draft error = %v, want ErrImmutable", err)
	}
}
