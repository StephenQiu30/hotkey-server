package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// MonitorEntityToDTO converts a KeywordMonitor entity to a Monitor DTO.
func MonitorEntityToDTO(m entity.KeywordMonitor) dto.Monitor {
	return dto.Monitor{
		ID:                   m.ID,
		UserID:               m.UserID,
		Name:                 m.Name,
		QueryText:            m.QueryText,
		Language:             m.Language,
		Region:               m.Region,
		Status:               m.Status,
		PollIntervalMinutes:  m.PollIntervalMinutes,
		AlertEnabled:         m.AlertEnabled,
		AlertThresholdConfig: m.AlertThresholdConfig.Data,
		LastPolledAt:         m.LastPolledAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// MonitorDTOToVO converts a Monitor DTO to a MonitorData VO.
func MonitorDTOToVO(m dto.Monitor) vo.MonitorData {
	return vo.MonitorData{
		ID:                  m.ID,
		UserID:              m.UserID,
		Name:                m.Name,
		QueryText:           m.QueryText,
		Language:            m.Language,
		Region:              m.Region,
		Status:              m.Status,
		PollIntervalMinutes: m.PollIntervalMinutes,
		AlertEnabled:        m.AlertEnabled,
	}
}

// MonitorSliceDTOToVO converts a slice of Monitor DTO to a slice of MonitorData VO.
func MonitorSliceDTOToVO(ms []dto.Monitor) []vo.MonitorData {
	result := make([]vo.MonitorData, len(ms))
	for i, m := range ms {
		result[i] = MonitorDTOToVO(m)
	}
	return result
}
