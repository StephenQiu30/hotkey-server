package repository

import (
	"context"
	"errors"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"gorm.io/gorm"
)

type ReportRepo struct {
	db *gorm.DB
}

func NewReportRepo(db *gorm.DB) *ReportRepo {
	return &ReportRepo{db: db}
}

func (r *ReportRepo) ListUserMonitors(ctx context.Context, userID int64) ([]dto.MonitorSource, error) {
	var rows []struct {
		ID     int64
		UserID int64
		Name   string
	}
	if err := r.db.WithContext(ctx).
		Table("keyword_monitors").
		Select("id, user_id, name").
		Where("user_id = ? AND status = ?", userID, "active").
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]dto.MonitorSource, len(rows))
	for i, row := range rows {
		out[i] = dto.MonitorSource{ID: row.ID, UserID: row.UserID, Name: row.Name}
	}
	return out, nil
}

func (r *ReportRepo) ListTopics(ctx context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]dto.TopicSource, error) {
	if len(monitorIDs) == 0 {
		return nil, nil
	}
	endExclusive := end.AddDate(0, 0, 1)
	var rows []struct {
		ID        int64
		MonitorID int64
		Title     string
		Summary   string
		HeatScore float64
		Trend     string
		PostCount int
	}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT t.id, t.monitor_id, t.title, t.summary, t.current_heat_score AS heat_score,
		        t.trend_direction AS trend, COUNT(tp.id) AS post_count
		 FROM topics t
		 LEFT JOIN topic_posts tp ON tp.topic_id = t.id
		 WHERE t.monitor_id IN ?
		   AND t.status = 'active'
		   AND t.last_active_at >= ?
		   AND t.last_active_at < ?
		 GROUP BY t.id
		 ORDER BY t.current_heat_score DESC
		 LIMIT ?`,
		monitorIDs, start, endExclusive, limit,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]dto.TopicSource, len(rows))
	for i, row := range rows {
		out[i] = dto.TopicSource{
			ID: row.ID, MonitorID: row.MonitorID, Title: row.Title, Summary: row.Summary,
			HeatScore: row.HeatScore, Trend: row.Trend, PostCount: row.PostCount,
		}
	}
	return out, nil
}

func (r *ReportRepo) ListPosts(ctx context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]dto.PostSource, error) {
	if len(monitorIDs) == 0 {
		return nil, nil
	}
	endExclusive := end.AddDate(0, 0, 1)
	var rows []struct {
		ID        int64
		MonitorID int64
		Content   string
		URL       string
		Platform  string
		HeatScore float64
	}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT pp.id, mph.monitor_id, pp.content_text AS content, pp.post_url AS url,
		        pp.platform, mph.final_score AS heat_score
		 FROM monitor_post_hits mph
		 JOIN platform_posts pp ON pp.id = mph.post_id
		 WHERE mph.monitor_id IN ?
		   AND (
		     (mph.first_seen_at >= ? AND mph.first_seen_at < ?)
		     OR (pp.published_at >= ? AND pp.published_at < ?)
		   )
		 ORDER BY mph.final_score DESC
		 LIMIT ?`,
		monitorIDs, start, endExclusive, start, endExclusive, limit,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]dto.PostSource, len(rows))
	for i, row := range rows {
		out[i] = dto.PostSource{
			ID: row.ID, MonitorID: row.MonitorID, Title: row.Content, Content: row.Content,
			URL: row.URL, Platform: row.Platform, HeatScore: row.HeatScore,
		}
	}
	return out, nil
}

func (r *ReportRepo) Create(ctx context.Context, in dto.CreateReportRecord) (dto.Report, error) {
	m := entity.Report{
		UserID:       in.UserID,
		ReportType:   in.ReportType,
		PeriodStart:  in.PeriodStart,
		PeriodEnd:    in.PeriodEnd,
		Subject:      in.Subject,
		Summary:      in.Summary,
		Content:      in.Content,
		HotspotCount: in.HotspotCount,
		Status:       in.Status,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return dto.Report{}, err
	}
	return toReport(m), nil
}

func (r *ReportRepo) List(ctx context.Context, filter dto.ListFilter) ([]dto.Report, int64, error) {
	query := r.db.WithContext(ctx).Model(&entity.Report{}).Where("user_id = ?", filter.UserID)
	if filter.ReportType != "" {
		query = query.Where("report_type = ?", filter.ReportType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var models []entity.Report
	if err := query.Order("created_at DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	out := make([]dto.Report, len(models))
	for i, m := range models {
		out[i] = toReport(m)
	}
	return out, total, nil
}

func (r *ReportRepo) GetByID(ctx context.Context, id, userID int64) (dto.Report, error) {
	var m entity.Report
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.Report{}, report.ErrNotFound
		}
		return dto.Report{}, err
	}
	return toReport(m), nil
}

func (r *ReportRepo) MarkSent(ctx context.Context, id, userID int64, sentAt time.Time) (dto.Report, error) {
	result := r.db.WithContext(ctx).Model(&entity.Report{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{
			"status":     report.StatusSent,
			"sent_at":    sentAt,
			"updated_at": sentAt,
		})
	if result.Error != nil {
		return dto.Report{}, result.Error
	}
	if result.RowsAffected == 0 {
		return dto.Report{}, report.ErrNotFound
	}
	return r.GetByID(ctx, id, userID)
}

func toReport(m entity.Report) dto.Report {
	return dto.Report{
		ID:           m.ID,
		UserID:       m.UserID,
		ReportType:   m.ReportType,
		PeriodStart:  m.PeriodStart,
		PeriodEnd:    m.PeriodEnd,
		Subject:      m.Subject,
		Summary:      m.Summary,
		Content:      m.Content,
		HotspotCount: m.HotspotCount,
		Status:       m.Status,
		SentAt:       m.SentAt,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

var _ report.Repository = (*ReportRepo)(nil)
