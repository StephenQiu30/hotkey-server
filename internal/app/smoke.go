package app

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

type smokeAuthRepo struct{ users []auth.User }

func (r *smokeAuthRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}

func (r *smokeAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	u := auth.User{
		ID: int64(len(r.users) + 1), Email: email, PasswordHash: passwordHash,
		DisplayName: displayName, Status: "active", PlanType: "free",
	}
	r.users = append(r.users, u)
	return u, nil
}

func (r *smokeAuthRepo) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}

func (r *smokeAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) { return nil, nil }

type smokeMonitorRepo struct{}

func (r *smokeMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}

func (r *smokeMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) { return nil, nil }

func (r *smokeMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}

func (r *smokeMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, monitor.ErrNotFound
}

type smokeNotifyRepo struct{}

func (r *smokeNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}

func (r *smokeNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }

func (r *smokeNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

type smokePostQueryService struct{}

func (s *smokePostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type smokeTopicQueryService struct{}

func (s *smokeTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type smokeTrendQueryService struct{}

func (s *smokeTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

func (s *smokeTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
