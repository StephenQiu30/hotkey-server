package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// HotEventEntityToDTO converts a HotEvent entity to a HotEvent DTO pointer.
func HotEventEntityToDTO(m entity.HotEvent) *dto.HotEvent {
	return &dto.HotEvent{
		ID:          m.ID,
		Name:        m.Name,
		HeatScore:   m.HeatScore,
		Platform:    m.Platform,
		Trend:       m.Trend,
		FirstSeenAt: m.FirstSeenAt,
		LastSeenAt:  m.LastSeenAt,
		PeakAt:      m.PeakAt,
		TopicIDs:    fromInt64Array(m.TopicIDs),
		PostIDs:     fromInt64Array(m.PostIDs),
		Summary:     m.Summary,
		Category:    m.Category,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// HotEventSliceEntityToDTO converts a slice of HotEvent entities to HotEvent DTO pointers.
func HotEventSliceEntityToDTO(models []entity.HotEvent) []*dto.HotEvent {
	events := make([]*dto.HotEvent, len(models))
	for i, m := range models {
		events[i] = HotEventEntityToDTO(m)
	}
	return events
}

func fromInt64Array(src pkg.Int64Array) []int64 {
	return []int64(src)
}
