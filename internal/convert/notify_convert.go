package convert

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// NotificationEntityToDTO converts a UserNotification entity to a Notification DTO.
func NotificationEntityToDTO(n entity.UserNotification) dto.Notification {
	return dto.Notification{
		ID:             n.ID,
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
		ReadAt:         n.ReadAt,
		SentAt:         n.SentAt,
		CreatedAt:      n.CreatedAt,
	}
}

// NotificationDTOToVO converts a Notification DTO to a NotificationData VO.
// time.Time fields are formatted as RFC3339 strings for JSON output.
func NotificationDTOToVO(n dto.Notification) vo.NotificationData {
	r := vo.NotificationData{
		ID:             n.ID,
		UserID:         n.UserID,
		AlertID:        n.AlertID,
		Channel:        n.Channel,
		DeliveryStatus: n.DeliveryStatus,
		CreatedAt:      n.CreatedAt.Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format(time.RFC3339)
		r.ReadAt = &s
	}
	return r
}

// NotificationSliceDTOToVO converts a slice of Notification DTO to NotificationData VOs.
func NotificationSliceDTOToVO(ns []dto.Notification) []vo.NotificationData {
	result := make([]vo.NotificationData, len(ns))
	for i, n := range ns {
		result[i] = NotificationDTOToVO(n)
	}
	return result
}
