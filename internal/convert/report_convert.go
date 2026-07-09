package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
)

// ReportExportEntityToDTO converts a ReportExport entity to a ReportExport DTO.
func ReportExportEntityToDTO(model entity.ReportExport) dto.ReportExport {
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
