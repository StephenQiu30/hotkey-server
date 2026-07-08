package fakenotify

import (
	"context"
	"sort"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
)

// Repo is an in-memory fake implementing notify.Repository.
type Repo struct {
	Notifications []dto.Notification
	nextID        int64
}

func (r *Repo) ListUnread(_ context.Context, userID int64) ([]dto.Notification, error) {
	var out []dto.Notification
	for _, n := range r.Notifications {
		if n.UserID == userID && n.ReadAt == nil {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (r *Repo) MarkRead(_ context.Context, userID, notificationID int64) error {
	for i := range r.Notifications {
		if r.Notifications[i].ID == notificationID {
			if r.Notifications[i].UserID != userID {
				return notify.ErrNotOwned
			}
			now := time.Now()
			r.Notifications[i].ReadAt = &now
			return nil
		}
	}
	return notify.ErrNotFound
}

func (r *Repo) Create(_ context.Context, n dto.Notification) (dto.Notification, error) {
	if r.nextID == 0 {
		for _, existing := range r.Notifications {
			if existing.ID >= r.nextID {
				r.nextID = existing.ID
			}
		}
	}
	r.nextID++
	n.ID = r.nextID
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	r.Notifications = append(r.Notifications, n)
	return n, nil
}
