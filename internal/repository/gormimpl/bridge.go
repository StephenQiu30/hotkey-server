package gormimpl

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────
// Auth bridge: gormimpl → auth.Repository
// ──────────────────────────────────────────────

type AuthRepoAdapter struct {
	db *gorm.DB
}

func NewAuthRepoAdapter(db *gorm.DB) *AuthRepoAdapter {
	return &AuthRepoAdapter{db: db}
}

func (r *AuthRepoAdapter) ExistsByEmail(ctx context.Context, email string) bool {
	u, err := NewUserRepo(r.db).GetByEmail(ctx, email)
	return err == nil && u != nil
}

func (r *AuthRepoAdapter) Create(ctx context.Context, email, passwordHash, displayName string) (auth.User, error) {
	u, err := NewUserRepo(r.db).Create(ctx, email, passwordHash, displayName)
	if err != nil {
		return auth.User{}, err
	}
	return auth.User{
		ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash,
		DisplayName: u.DisplayName, Status: u.Status, PlanType: u.PlanType,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}, nil
}

func (r *AuthRepoAdapter) GetByEmail(ctx context.Context, email string) (*auth.User, error) {
	u, err := NewUserRepo(r.db).GetByEmail(ctx, email)
	if err != nil || u == nil {
		return nil, err
	}
	return &auth.User{
		ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash,
		DisplayName: u.DisplayName, Status: u.Status, PlanType: u.PlanType,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}, nil
}

func (r *AuthRepoAdapter) GetByID(ctx context.Context, id int64) (*auth.User, error) {
	u, err := NewUserRepo(r.db).GetByID(ctx, id)
	if err != nil || u == nil {
		return nil, err
	}
	return &auth.User{
		ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash,
		DisplayName: u.DisplayName, Status: u.Status, PlanType: u.PlanType,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}, nil
}

var _ auth.Repository = (*AuthRepoAdapter)(nil)

// ──────────────────────────────────────────────
// Monitor bridge: gormimpl → monitor.Repository
// ──────────────────────────────────────────────

type MonitorRepoAdapter struct {
	db *gorm.DB
}

func NewMonitorRepoAdapter(db *gorm.DB) *MonitorRepoAdapter {
	return &MonitorRepoAdapter{db: db}
}

func (r *MonitorRepoAdapter) Create(ctx context.Context, userID int64, input monitor.CreateMonitorInput) (monitor.Monitor, error) {
	im := modelKeyMon{
		UserID: userID, Name: input.Name, QueryText: input.QueryText,
		Language: input.Language, Region: input.Region,
		PollIntervalMinutes: input.PollIntervalMinutes, AlertEnabled: input.AlertEnabled,
		AlertThresholdConfig: pkg.JSONB[map[string]any]{Data: make(map[string]any)},
	}
	m := KeywordMonitor{
		UserID: im.UserID, Name: im.Name, QueryText: im.QueryText,
		Language: im.Language, Region: im.Region,
		PollIntervalMinutes: im.PollIntervalMinutes, AlertEnabled: im.AlertEnabled,
		AlertThresholdConfig: im.AlertThresholdConfig,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return monitor.Monitor{}, err
	}
	return monToDomain(m), nil
}

func (r *MonitorRepoAdapter) GetByID(ctx context.Context, id int64) (*monitor.Monitor, error) {
	var m KeywordMonitor
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	result := monToDomain(m)
	return &result, nil
}

func (r *MonitorRepoAdapter) ListByUser(ctx context.Context, userID int64) ([]monitor.Monitor, error) {
	var models []KeywordMonitor
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]monitor.Monitor, len(models))
	for i := range models {
		result[i] = monToDomain(models[i])
	}
	return result, nil
}

func (r *MonitorRepoAdapter) Update(ctx context.Context, id int64, userID int64, input monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	updates := make(map[string]any)
	if input.Name != nil { updates["name"] = *input.Name }
	if input.QueryText != nil { updates["query_text"] = *input.QueryText }
	if input.Language != nil { updates["language"] = *input.Language }
	if input.Region != nil { updates["region"] = *input.Region }
	if input.PollIntervalMinutes != nil { updates["poll_interval_minutes"] = *input.PollIntervalMinutes }
	if input.AlertEnabled != nil { updates["alert_enabled"] = *input.AlertEnabled }
	if input.Status != nil { updates["status"] = *input.Status }
	if len(updates) == 0 {
		got, err := r.GetByID(ctx, id)
		if err != nil || got == nil {
			return monitor.Monitor{}, err
		}
		return *got, nil
	}
	updates["updated_at"] = "now()"
	if err := r.db.WithContext(ctx).Model(&KeywordMonitor{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates).Error; err != nil {
		return monitor.Monitor{}, err
	}
	got, err := r.GetByID(ctx, id)
	if err != nil || got == nil {
		return monitor.Monitor{}, err
	}
	return *got, nil
}

func monToDomain(m KeywordMonitor) monitor.Monitor {
	return monitor.Monitor{
		ID: m.ID, UserID: m.UserID, Name: m.Name, QueryText: m.QueryText,
		Language: m.Language, Region: m.Region, Status: m.Status,
		PollIntervalMinutes: m.PollIntervalMinutes, AlertEnabled: m.AlertEnabled,
		AlertThresholdConfig: m.AlertThresholdConfig.Data,
		LastPolledAt: m.LastPolledAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

type modelKeyMon struct {
	UserID               int64
	Name                 string
	QueryText            string
	Language             string
	Region               string
	PollIntervalMinutes  int
	AlertEnabled         bool
	AlertThresholdConfig pkg.JSONB[map[string]any]
}

var _ monitor.Repository = (*MonitorRepoAdapter)(nil)

// ──────────────────────────────────────────────
// Notify bridge: gormimpl → notify.Repository
// ──────────────────────────────────────────────

type NotifyRepoAdapter struct {
	db *gorm.DB
}

func NewNotifyRepoAdapter(db *gorm.DB) *NotifyRepoAdapter {
	return &NotifyRepoAdapter{db: db}
}

func (r *NotifyRepoAdapter) ListUnread(ctx context.Context, userID int64) ([]notify.Notification, error) {
	var models []UserNotification
	if err := r.db.WithContext(ctx).Where("user_id = ? AND read_at IS NULL", userID).
		Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]notify.Notification, len(models))
	for i := range models {
		result[i] = notify.Notification{
			ID: models[i].ID, UserID: models[i].UserID, AlertID: models[i].AlertID,
			Channel: models[i].Channel, DeliveryStatus: models[i].DeliveryStatus,
			ReadAt: models[i].ReadAt, SentAt: models[i].SentAt, CreatedAt: models[i].CreatedAt,
		}
	}
	return result, nil
}

func (r *NotifyRepoAdapter) MarkRead(ctx context.Context, userID, notificationID int64) error {
	result := r.db.WithContext(ctx).Model(&UserNotification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Update("read_at", gorm.Expr("now()"))
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}

func (r *NotifyRepoAdapter) Create(ctx context.Context, n notify.Notification) (notify.Notification, error) {
	m := UserNotification{
		UserID: n.UserID, AlertID: n.AlertID, Channel: n.Channel,
		DeliveryStatus: n.DeliveryStatus,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return notify.Notification{}, err
	}
	n.ID = m.ID
	n.CreatedAt = m.CreatedAt
	return n, nil
}

var _ notify.Repository = (*NotifyRepoAdapter)(nil)
