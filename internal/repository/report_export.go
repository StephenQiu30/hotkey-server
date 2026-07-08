package repository

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"gorm.io/gorm"
)

type ReportExportRepo struct {
	db *gorm.DB
}

func NewReportExportRepo(db *gorm.DB) *ReportExportRepo {
	return &ReportExportRepo{db: db}
}

func (r *ReportExportRepo) CreatePending(ctx context.Context, input dto.CreateReportExportInput) (dto.ReportExport, error) {
	model := entity.ReportExport{
		ReportID:   input.ReportID,
		ExportKind: input.ExportKind,
		TargetPath: input.TargetPath,
		Status:     dto.ExportStatusPending,
	}
	err := r.db.WithContext(ctx).Where(entity.ReportExport{
		ReportID:   input.ReportID,
		ExportKind: input.ExportKind,
	}).Attrs(model).FirstOrCreate(&model).Error
	if err != nil {
		return dto.ReportExport{}, err
	}
	return toDTOReportExport(model), nil
}

func (r *ReportExportRepo) MarkPublished(ctx context.Context, reportID int64, exportKind string, path string, publishedAt time.Time) (dto.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, dto.ExportStatusPublished, "", &publishedAt, publishedAt)
}

func (r *ReportExportRepo) MarkSkipped(ctx context.Context, reportID int64, exportKind string, path string, skippedAt time.Time) (dto.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, dto.ExportStatusSkipped, "", nil, skippedAt)
}

func (r *ReportExportRepo) MarkFailed(ctx context.Context, reportID int64, exportKind string, path string, message string, failedAt time.Time) (dto.ReportExport, error) {
	return r.updateStatus(ctx, reportID, exportKind, path, dto.ExportStatusFailed, message, nil, failedAt)
}

func (r *ReportExportRepo) ListByReport(ctx context.Context, reportID int64) ([]dto.ReportExport, error) {
	var models []entity.ReportExport
	if err := r.db.WithContext(ctx).Where("report_id = ?", reportID).Order("export_kind ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]dto.ReportExport, len(models))
	for i, model := range models {
		out[i] = toDTOReportExport(model)
	}
	return out, nil
}

func (r *ReportExportRepo) updateStatus(ctx context.Context, reportID int64, exportKind string, path string, status string, message string, publishedAt *time.Time, updatedAt time.Time) (dto.ReportExport, error) {
	model := entity.ReportExport{}
	err := r.db.WithContext(ctx).Where(entity.ReportExport{
		ReportID:   reportID,
		ExportKind: exportKind,
	}).Attrs(entity.ReportExport{
		TargetPath: path,
	}).FirstOrCreate(&model).Error
	if err != nil {
		return dto.ReportExport{}, err
	}

	updates := map[string]any{
		"target_path":   path,
		"status":        status,
		"error_message": message,
		"published_at":  publishedAt,
		"updated_at":    updatedAt,
	}
	if err := r.db.WithContext(ctx).Model(&model).Updates(updates).Error; err != nil {
		return dto.ReportExport{}, err
	}
	return r.ListOne(ctx, reportID, exportKind)
}

func (r *ReportExportRepo) ListOne(ctx context.Context, reportID int64, exportKind string) (dto.ReportExport, error) {
	var model entity.ReportExport
	if err := r.db.WithContext(ctx).Where("report_id = ? AND export_kind = ?", reportID, exportKind).First(&model).Error; err != nil {
		return dto.ReportExport{}, err
	}
	return toDTOReportExport(model), nil
}

func toDTOReportExport(model entity.ReportExport) dto.ReportExport {
	return dto.ReportExport{
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
