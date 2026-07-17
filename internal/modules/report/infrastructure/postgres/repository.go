package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
		err := transaction.SQL.QueryRowContext(ctx, `SELECT status FROM reports WHERE id = $1 FOR UPDATE`, report.ID).Scan(&existingStatus)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return sharedrepository.MapError(err)
		}
		if err == nil && existingStatus == string(domain.ReportPublished) {
			return sharedrepository.ErrImmutable
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

func (repository *Repository) Get(ctx context.Context, reportID int64) (domain.Report, error) {
	if repository == nil || repository.runtime == nil || reportID <= 0 {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	queryer := reportQueryerFor(ctx, repository.runtime)
	report, err := scanReport(queryer.QueryRowContext(ctx, reportSelect+` WHERE id = $1 AND deleted_at IS NULL`, reportID))
	if err != nil {
		return domain.Report{}, err
	}
	items, err := repository.items(ctx, queryer, report.ID)
	if err != nil {
		return domain.Report{}, err
	}
	report.Items = items
	return report, nil
}

func (repository *Repository) List(ctx context.Context, query domain.ListQuery) (domain.Page, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Page{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.Page{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	reportType, status := "", ""
	if query.Type != nil {
		reportType = string(*query.Type)
	}
	if query.Status != nil {
		status = string(*query.Status)
	}
	rows, err := reportQueryerFor(ctx, repository.runtime).QueryContext(ctx, reportSelect+`
WHERE deleted_at IS NULL
  AND ($1 = '' OR report_type = $1)
  AND ($2 = '' OR status = $2)
  AND ($3 = 0 OR id < $3)
ORDER BY id DESC
LIMIT $4`, reportType, status, query.Cursor, query.Limit+1)
	if err != nil {
		return domain.Page{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	page := domain.Page{Items: make([]domain.Report, 0, query.Limit)}
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return domain.Page{}, err
		}
		page.Items = append(page.Items, report)
	}
	if err := rows.Err(); err != nil {
		return domain.Page{}, sharedrepository.MapError(err)
	}
	if len(page.Items) > query.Limit {
		page.NextCursor = page.Items[query.Limit-1].ID
		page.Items = page.Items[:query.Limit]
	}
	return page, nil
}

const reportSelect = `SELECT id, version, report_type, monitor_id, period_start, period_end, timezone, title, summary, body, status, version_no, generated_at, published_at FROM reports`

type reportRow interface {
	Scan(...any) error
}

type reportQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func reportQueryerFor(ctx context.Context, runtime *database.Runtime) reportQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}

func scanReport(row reportRow) (domain.Report, error) {
	var report domain.Report
	var reportType, status string
	var monitorID sql.NullInt64
	var generatedAt, publishedAt sql.NullTime
	if err := row.Scan(&report.ID, &report.Version, &reportType, &monitorID, &report.Period.Start, &report.Period.End, &reportTimezone{period: &report.Period}, &report.Title, &report.Summary, &report.Body, &status, &report.VersionNo, &generatedAt, &publishedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Report{}, sharedrepository.ErrNotFound
		}
		return domain.Report{}, sharedrepository.MapError(err)
	}
	report.Type, report.Status = domain.ReportType(reportType), domain.ReportStatus(status)
	report.Frozen = report.Status == domain.ReportPublished
	if monitorID.Valid {
		value := monitorID.Int64
		report.MonitorID = &value
	}
	if generatedAt.Valid {
		value := generatedAt.Time.UTC()
		report.GeneratedAt = &value
	}
	if publishedAt.Valid {
		value := publishedAt.Time.UTC()
		report.PublishedAt = &value
	}
	return report, nil
}

// reportTimezone restores the location used when the report period was
// calculated. The database keeps timestamps in UTC while the report contract
// keeps its original calendar timezone for display and future versioning.
type reportTimezone struct{ period *domain.Period }

func (target *reportTimezone) Scan(value any) error {
	var name string
	switch typed := value.(type) {
	case string:
		name = typed
	case []byte:
		name = string(typed)
	default:
		return fmt.Errorf("invalid report timezone")
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("invalid report timezone: %w", err)
	}
	target.period.Location = location
	return nil
}

func (repository *Repository) items(ctx context.Context, queryer reportQueryer, reportID int64) ([]domain.Item, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT event_id, rank, inclusion_reason, title_snapshot, summary_snapshot, heat_score_snapshot FROM report_items WHERE report_id = $1 ORDER BY rank, event_id`, reportID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.Item, 0)
	for rows.Next() {
		var item domain.Item
		if err := rows.Scan(&item.EventID, &item.Rank, &item.InclusionReason, &item.Title, &item.Summary, &item.HeatScore); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return items, nil
}
