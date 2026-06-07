package reportrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveReport(ctx context.Context, report servicereport.DailyReport) (servicereport.DailyReport, error) {
	const query = `
INSERT INTO daily_reports (
	id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (date, channel_id, user_id) DO UPDATE SET
	prompt_version = EXCLUDED.prompt_version,
	input_hotspot_ids_json = EXCLUDED.input_hotspot_ids_json,
	body = EXCLUDED.body,
	status = EXCLUDED.status,
	last_error = EXCLUDED.last_error,
	source_refs_json = EXCLUDED.source_refs_json,
	updated_at = EXCLUDED.updated_at
RETURNING id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at`

	hotspotIDsJSON, _ := json.Marshal(report.InputHotspotIDs)
	sourceRefsJSON, _ := json.Marshal(report.SourceRefs)

	var date time.Time
	var hotspotIDsRaw, sourceRefsRaw []byte
	err := r.db.QueryRowContext(ctx, query,
		report.ID, report.Date, report.ChannelID, report.UserID, report.PromptVersion, hotspotIDsJSON, report.Body, string(report.Status), report.LastError, sourceRefsJSON, report.CreatedAt, report.UpdatedAt,
	).Scan(
		&report.ID, &date, &report.ChannelID, &report.UserID, &report.PromptVersion, &hotspotIDsRaw, &report.Body, &report.Status, &report.LastError, &sourceRefsRaw, &report.CreatedAt, &report.UpdatedAt,
	)
	if err != nil {
		return servicereport.DailyReport{}, err
	}
	report.Date = date.Format("2006-01-02")
	_ = json.Unmarshal(hotspotIDsRaw, &report.InputHotspotIDs)
	_ = json.Unmarshal(sourceRefsRaw, &report.SourceRefs)
	return report, nil
}

func (r *Repository) FindReportByDateChannelUser(ctx context.Context, date, channelID, userID string) (servicereport.DailyReport, error) {
	const query = `
SELECT id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at
FROM daily_reports
WHERE date = $1 AND channel_id = $2 AND user_id = $3`
	var report servicereport.DailyReport
	var dateVal time.Time
	var hotspotIDsRaw, sourceRefsRaw []byte
	err := r.db.QueryRowContext(ctx, query, date, channelID, userID).Scan(
		&report.ID, &dateVal, &report.ChannelID, &report.UserID, &report.PromptVersion, &hotspotIDsRaw, &report.Body, &report.Status, &report.LastError, &sourceRefsRaw, &report.CreatedAt, &report.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return servicereport.DailyReport{}, servicereport.ErrNotFound
	}
	if err != nil {
		return servicereport.DailyReport{}, err
	}
	report.Date = dateVal.Format("2006-01-02")
	_ = json.Unmarshal(hotspotIDsRaw, &report.InputHotspotIDs)
	_ = json.Unmarshal(sourceRefsRaw, &report.SourceRefs)
	return report, nil
}

func (r *Repository) FindReportByID(ctx context.Context, id string) (servicereport.DailyReport, error) {
	const query = `
SELECT id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at
FROM daily_reports
WHERE id = $1`
	var report servicereport.DailyReport
	var dateVal time.Time
	var hotspotIDsRaw, sourceRefsRaw []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&report.ID, &dateVal, &report.ChannelID, &report.UserID, &report.PromptVersion, &hotspotIDsRaw, &report.Body, &report.Status, &report.LastError, &sourceRefsRaw, &report.CreatedAt, &report.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return servicereport.DailyReport{}, servicereport.ErrNotFound
	}
	if err != nil {
		return servicereport.DailyReport{}, err
	}
	report.Date = dateVal.Format("2006-01-02")
	_ = json.Unmarshal(hotspotIDsRaw, &report.InputHotspotIDs)
	_ = json.Unmarshal(sourceRefsRaw, &report.SourceRefs)
	return report, nil
}

func (r *Repository) ListReportsByDate(ctx context.Context, date string) ([]servicereport.DailyReport, error) {
	const query = `
SELECT id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at
FROM daily_reports
WHERE date = $1
ORDER BY created_at DESC`
	return r.listReports(ctx, query, date)
}

func (r *Repository) ListReportsByChannel(ctx context.Context, channelID string) ([]servicereport.DailyReport, error) {
	const query = `
SELECT id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at
FROM daily_reports
WHERE channel_id = $1
ORDER BY date DESC`
	return r.listReports(ctx, query, channelID)
}

func (r *Repository) ListReportsByUser(ctx context.Context, userID string) ([]servicereport.DailyReport, error) {
	const query = `
SELECT id, date, channel_id, user_id, prompt_version, input_hotspot_ids_json, body, status, last_error, source_refs_json, created_at, updated_at
FROM daily_reports
WHERE user_id = $1
ORDER BY date DESC`
	return r.listReports(ctx, query, userID)
}

func (r *Repository) listReports(ctx context.Context, query string, arg any) ([]servicereport.DailyReport, error) {
	rows, err := r.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []servicereport.DailyReport
	for rows.Next() {
		var report servicereport.DailyReport
		var dateVal time.Time
		var hotspotIDsRaw, sourceRefsRaw []byte
		if err := rows.Scan(
			&report.ID, &dateVal, &report.ChannelID, &report.UserID, &report.PromptVersion, &hotspotIDsRaw, &report.Body, &report.Status, &report.LastError, &sourceRefsRaw, &report.CreatedAt, &report.UpdatedAt,
		); err != nil {
			return nil, err
		}
		report.Date = dateVal.Format("2006-01-02")
		_ = json.Unmarshal(hotspotIDsRaw, &report.InputHotspotIDs)
		_ = json.Unmarshal(sourceRefsRaw, &report.SourceRefs)
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (r *Repository) SaveSummary(ctx context.Context, summary servicereport.AISummary) (servicereport.AISummary, error) {
	const query = `
INSERT INTO ai_summaries (
	id, cluster_id, prompt_version, summary, status, last_error, source_refs_json, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (cluster_id, prompt_version) DO UPDATE SET
	summary = EXCLUDED.summary,
	status = EXCLUDED.status,
	last_error = EXCLUDED.last_error,
	source_refs_json = EXCLUDED.source_refs_json,
	updated_at = EXCLUDED.updated_at
RETURNING id, cluster_id, prompt_version, summary, status, last_error, source_refs_json, created_at, updated_at`

	sourceRefsJSON, _ := json.Marshal(summary.SourceRefs)

	var sourceRefsRaw []byte
	err := r.db.QueryRowContext(ctx, query,
		summary.ID, summary.ClusterID, summary.PromptVersion, summary.Summary, string(summary.Status), summary.LastError, sourceRefsJSON, summary.CreatedAt, summary.UpdatedAt,
	).Scan(
		&summary.ID, &summary.ClusterID, &summary.PromptVersion, &summary.Summary, &summary.Status, &summary.LastError, &sourceRefsRaw, &summary.CreatedAt, &summary.UpdatedAt,
	)
	if err != nil {
		return servicereport.AISummary{}, err
	}
	_ = json.Unmarshal(sourceRefsRaw, &summary.SourceRefs)
	return summary, nil
}

func (r *Repository) FindSummaryByClusterID(ctx context.Context, clusterID string) (servicereport.AISummary, error) {
	const query = `
SELECT id, cluster_id, prompt_version, summary, status, last_error, source_refs_json, created_at, updated_at
FROM ai_summaries
WHERE cluster_id = $1
ORDER BY updated_at DESC
LIMIT 1`
	var summary servicereport.AISummary
	var sourceRefsRaw []byte
	err := r.db.QueryRowContext(ctx, query, clusterID).Scan(
		&summary.ID, &summary.ClusterID, &summary.PromptVersion, &summary.Summary, &summary.Status, &summary.LastError, &sourceRefsRaw, &summary.CreatedAt, &summary.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return servicereport.AISummary{}, servicereport.ErrNotFound
	}
	if err != nil {
		return servicereport.AISummary{}, err
	}
	_ = json.Unmarshal(sourceRefsRaw, &summary.SourceRefs)
	return summary, nil
}
