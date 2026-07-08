package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"gorm.io/gorm"
)

type ReportExportRepo struct {
	db *gorm.DB
}

func NewReportExportRepo(db *gorm.DB) *ReportExportRepo {
	return &ReportExportRepo{db: db}
}

func (r *ReportExportRepo) CreatePending(ctx context.Context, input report.CreateReportExportInput) (report.ReportExport, error) {
	model := entity.ReportExport{
		ReportID:   input.ReportID,
		ExportKind: input.ExportKind,
		TargetPath: input.TargetPath,
		Status:     report.ExportStatusPending,
	}
	err := r.db.WithContext(ctx).Where(entity.ReportExport{
		ReportID:   input.ReportID,
		ExportKind: input.ExportKind,
	}).Attrs(model).FirstOrCreate(&model).Error
	if err != nil {
		return report.ReportExport{}, err
	}
	return toReportExport(model), nil
}

func (r *ReportExportRepo) MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (report.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, report.ExportStatusPublished, "", &publishedAt, publishedAt)
}

func (r *ReportExportRepo) MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (report.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, report.ExportStatusSkipped, "", nil, skippedAt)
}

func (r *ReportExportRepo) MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (report.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, report.ExportStatusFailed, message, nil, failedAt)
}

func (r *ReportExportRepo) ListByReport(ctx context.Context, reportID int64) ([]report.ReportExport, error) {
	var models []entity.ReportExport
	if err := r.db.WithContext(ctx).Where("report_id = ?", reportID).Order("export_kind ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]report.ReportExport, len(models))
	for i, model := range models {
		out[i] = toReportExport(model)
	}
	return out, nil
}

func (r *ReportExportRepo) updateStatus(ctx context.Context, reportID int64, exportKind string, path string, status string, message string, publishedAt *time.Time, updatedAt time.Time) (report.ReportExport, error) {
	model := entity.ReportExport{}
	err := r.db.WithContext(ctx).Where(entity.ReportExport{
		ReportID:   reportID,
		ExportKind: exportKind,
	}).Attrs(entity.ReportExport{
		TargetPath: path,
	}).FirstOrCreate(&model).Error
	if err != nil {
		return report.ReportExport{}, err
	}

	updates := map[string]any{
		"target_path":   path,
		"status":        status,
		"error_message": message,
		"published_at":  publishedAt,
		"updated_at":    updatedAt,
	}
	if err := r.db.WithContext(ctx).Model(&model).Updates(updates).Error; err != nil {
		return report.ReportExport{}, err
	}
	return r.ListOne(ctx, reportID, exportKind)
}

func (r *ReportExportRepo) ListOne(ctx context.Context, reportID int64, exportKind string) (report.ReportExport, error) {
	var model entity.ReportExport
	if err := r.db.WithContext(ctx).Where("report_id = ? AND export_kind = ?", reportID, exportKind).First(&model).Error; err != nil {
		return report.ReportExport{}, err
	}
	return toReportExport(model), nil
}

func toReportExport(model entity.ReportExport) report.ReportExport {
	return report.ReportExport{
		ID:           model.ID,
		ReportID:     model.ReportID,
		ExportKind:   model.ExportKind,
		TargetPath:   model.TargetPath,
		Status:       model.Status,
		ErrorMessage: model.ErrorMessage,
		PublishedAt:  model.PublishedAt,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

var _ report.ExportRepository = (*ReportExportRepo)(nil)
