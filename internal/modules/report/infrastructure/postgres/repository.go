package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) Save(ctx context.Context, report domain.Report) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := report.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return repository.runtime.WithinTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var existingStatus string
		var existingVersion int64
		err := transaction.SQL.QueryRowContext(ctx, `SELECT status, version FROM reports WHERE id = $1 FOR UPDATE`, report.ID).Scan(&existingStatus, &existingVersion)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return sharedrepository.MapError(err)
		}
		if err == nil && existingStatus == string(domain.ReportPublished) {
			if report.Status != domain.ReportPublished || report.Version <= existingVersion {
				return sharedrepository.ErrImmutable
			}
		}
		if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO reports (id, version, report_type, monitor_id, period_start, period_end, timezone, title, summary, body, status, version_no, generated_at, published_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),CASE WHEN $13::text = 'published' THEN now() ELSE NULL END) ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, summary = EXCLUDED.summary, body = EXCLUDED.body, status = EXCLUDED.status, published_at = EXCLUDED.published_at`, report.ID, report.Version, report.Type, report.MonitorID, report.Period.Start.UTC(), report.Period.End.UTC(), report.Period.Location.String(), report.Title, report.Summary, report.Body, report.Status, report.VersionNo, report.Status); err != nil {
			return sharedrepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `DELETE FROM report_items WHERE report_id = $1`, report.ID); err != nil {
			return sharedrepository.MapError(err)
		}
		for _, item := range report.Items {
			if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO report_items (report_id, event_id, rank, section, inclusion_reason, title_snapshot, summary_snapshot, heat_score_snapshot) VALUES ($1,$2,$3,'events',$4,$5,$6,$7) ON CONFLICT (report_id, event_id) DO UPDATE SET rank = EXCLUDED.rank, title_snapshot = EXCLUDED.title_snapshot, summary_snapshot = EXCLUDED.summary_snapshot, heat_score_snapshot = EXCLUDED.heat_score_snapshot`, report.ID, item.EventID, item.Rank, item.InclusionReason, item.Title, item.Summary, item.HeatScore); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		return nil
	})
}
