package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type ComponentStatus string

const (
	ComponentStatusOK       ComponentStatus = "ok"
	ComponentStatusDegraded ComponentStatus = "degraded"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
)

type AuditLog struct {
	ID           string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	Metadata     map[string]string
	CreatedAt    time.Time
}

type AuditLogInput struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	Metadata     map[string]string
}

type CleanupStatus string

const (
	CleanupStatusPending    CleanupStatus = "pending"
	CleanupStatusInProgress CleanupStatus = "in_progress"
	CleanupStatusCompleted  CleanupStatus = "completed"
	CleanupStatusFailed     CleanupStatus = "failed"
)

type CleanupStep struct {
	Name   string
	Status CleanupStatus
	Error  string
}

type CleanupTask struct {
	ID        string
	UserID    string
	Status    CleanupStatus
	Steps     []CleanupStep
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UserRecord struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	Status       string
}

type RSSFeedRecord struct {
	UserID    string
	TokenHash string
	Enabled   bool
}

type DailyReportRecord struct {
	ID     string
	UserID string
	Date   string
}

type CreateJobInput struct {
	Type           queue.JobType
	Payload        json.RawMessage
	Status         queue.JobStatus
	Attempt        int
	MaxAttempts    int
	IdempotencyKey string
	LastError      string
	NextRunAt      time.Time
}

type RerunDailyReportInput struct {
	Date      string
	ChannelID string
	UserID    string
}

type QueueOverview struct {
	Total      int
	Pending    int
	Running    int
	Scheduled  int
	Succeeded  int
	Failed     int
	DeadLetter int
}

type ComponentCheck struct {
	Status ComponentStatus
	Reason string
}

type ConfigStatus struct {
	Overall    ComponentStatus
	Components map[string]ComponentCheck
}

type Repository interface {
	CreateAuditLog(ctx context.Context, entry AuditLog) (AuditLog, error)
	ListAuditLogs(ctx context.Context) ([]AuditLog, error)
	CreateJob(ctx context.Context, job queue.Job) (queue.Job, error)
	ListJobs(ctx context.Context) ([]queue.Job, error)
	JobByID(ctx context.Context, jobID string) (queue.Job, error)
	UpdateJob(ctx context.Context, job queue.Job) (queue.Job, error)
	UserByID(ctx context.Context, userID string) (UserRecord, error)
	DeleteUser(ctx context.Context, userID string) error
	DeleteRSSFeedByUser(ctx context.Context, userID string) error
	DeleteDailyReportsByUser(ctx context.Context, userID string) error
	SaveCleanupTask(ctx context.Context, task CleanupTask) error
	CleanupTaskByID(ctx context.Context, taskID string) (CleanupTask, error)
}

type RevokedSource struct {
	ID        string
	Name      string
	Status    string
	UpdatedAt time.Time
}

type Config struct {
	PostgreSQLPing   func(context.Context) error
	RedisPing        func(context.Context) error
	DashScopeKey     string
	SMTPHost         string
	Now              func() time.Time
	RevokeSourceFunc func(ctx context.Context, sourceID string) (RevokedSource, error)
}

type Service struct {
	repo Repository
	cfg  Config
	now  func() time.Time
}

func NewService(repo Repository, cfg Config) *Service {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, cfg: cfg, now: now}
}

var sensitiveFieldPatterns = []string{
	"token", "password", "secret", "api_key", "apikey", "credential",
}

func (s *Service) RecordAuditLog(ctx context.Context, input AuditLogInput) (AuditLog, error) {
	if strings.TrimSpace(input.ActorID) == "" || strings.TrimSpace(input.Action) == "" || strings.TrimSpace(input.ResourceType) == "" || strings.TrimSpace(input.Result) == "" {
		return AuditLog{}, ErrInvalidInput
	}
	return s.repo.CreateAuditLog(ctx, AuditLog{
		ID:           newID("aud"),
		ActorID:      strings.TrimSpace(input.ActorID),
		Action:       strings.TrimSpace(input.Action),
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.TrimSpace(input.ResourceID),
		Result:       strings.TrimSpace(input.Result),
		Metadata:     sanitizeMetadata(input.Metadata),
		CreatedAt:    s.now().UTC(),
	})
}

func sanitizeMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	sanitized := make(map[string]string, len(metadata))
	for k, v := range metadata {
		if isSensitiveField(k) {
			sanitized[k] = "[REDACTED]"
		} else {
			sanitized[k] = v
		}
	}
	return sanitized
}

func isSensitiveField(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range sensitiveFieldPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func (s *Service) ListAuditLogs(ctx context.Context) ([]AuditLog, error) {
	return s.repo.ListAuditLogs(ctx)
}

func (s *Service) CreateJob(ctx context.Context, input CreateJobInput) (queue.Job, error) {
	if input.Status == "" {
		input.Status = queue.JobStatusPending
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = 3
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = fmt.Sprintf("%s:%d", input.Type, s.now().UnixNano())
	}
	if err := queue.ValidatePayload(input.Type, input.Payload); err != nil {
		return queue.Job{}, err
	}
	now := s.now().UTC()
	nextRunAt := input.NextRunAt
	if nextRunAt.IsZero() {
		nextRunAt = now
	}
	return s.repo.CreateJob(ctx, queue.Job{
		ID:             newID("job"),
		Type:           input.Type,
		Payload:        append(json.RawMessage(nil), input.Payload...),
		Status:         input.Status,
		Attempt:        input.Attempt,
		MaxAttempts:    input.MaxAttempts,
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		LastError:      strings.TrimSpace(input.LastError),
		NextRunAt:      nextRunAt.UTC(),
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

func (s *Service) QueueOverview(ctx context.Context) (QueueOverview, error) {
	jobs, err := s.repo.ListJobs(ctx)
	if err != nil {
		return QueueOverview{}, err
	}
	var overview QueueOverview
	for _, job := range jobs {
		overview.Total++
		switch job.Status {
		case queue.JobStatusPending:
			overview.Pending++
		case queue.JobStatusRunning:
			overview.Running++
		case queue.JobStatusScheduled:
			overview.Scheduled++
		case queue.JobStatusSucceeded:
			overview.Succeeded++
		case queue.JobStatusFailed:
			overview.Failed++
		case queue.JobStatusDeadLetter:
			overview.DeadLetter++
		}
	}
	return overview, nil
}

func (s *Service) ListJobs(ctx context.Context) ([]queue.Job, error) {
	return s.repo.ListJobs(ctx)
}

func (s *Service) ListFailedJobs(ctx context.Context) ([]queue.Job, error) {
	jobs, err := s.repo.ListJobs(ctx)
	if err != nil {
		return nil, err
	}
	failed := make([]queue.Job, 0)
	for _, job := range jobs {
		if job.Status == queue.JobStatusFailed || job.Status == queue.JobStatusDeadLetter {
			failed = append(failed, job)
		}
	}
	return failed, nil
}

func (s *Service) JobByID(ctx context.Context, jobID string) (queue.Job, error) {
	if strings.TrimSpace(jobID) == "" {
		return queue.Job{}, ErrInvalidInput
	}
	return s.repo.JobByID(ctx, strings.TrimSpace(jobID))
}

func (s *Service) RetryJob(ctx context.Context, jobID string) (queue.Job, error) {
	job, err := s.JobByID(ctx, jobID)
	if err != nil {
		return queue.Job{}, err
	}
	if job.Status != queue.JobStatusFailed && job.Status != queue.JobStatusDeadLetter {
		return queue.Job{}, ErrInvalidInput
	}
	job.Status = queue.JobStatusPending
	job.Attempt = 0
	job.LastError = ""
	job.NextRunAt = s.now().UTC()
	job.UpdatedAt = job.NextRunAt
	return s.repo.UpdateJob(ctx, job)
}

func (s *Service) RerunDailyReport(ctx context.Context, input RerunDailyReportInput) (queue.Job, error) {
	date := strings.TrimSpace(input.Date)
	if date == "" {
		return queue.Job{}, ErrInvalidInput
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return queue.Job{}, ErrInvalidInput
	}
	payload, err := json.Marshal(map[string]string{
		"date":       date,
		"channel_id": strings.TrimSpace(input.ChannelID),
		"user_id":    strings.TrimSpace(input.UserID),
	})
	if err != nil {
		return queue.Job{}, err
	}
	return s.CreateJob(ctx, CreateJobInput{
		Type:           queue.JobTypeGenerateDailyReport,
		Payload:        payload,
		Status:         queue.JobStatusPending,
		IdempotencyKey: fmt.Sprintf("daily_report_rerun:%s:%s:%s", date, strings.TrimSpace(input.ChannelID), strings.TrimSpace(input.UserID)),
	})
}

func (s *Service) DeleteAccount(ctx context.Context, userID string) (CleanupTask, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return CleanupTask{}, ErrInvalidInput
	}
	if _, err := s.repo.UserByID(ctx, userID); err != nil {
		return CleanupTask{}, err
	}

	task := CleanupTask{
		ID:        newID("cleanup"),
		UserID:    userID,
		Status:    CleanupStatusInProgress,
		Steps:     make([]CleanupStep, 0),
		CreatedAt: s.now().UTC(),
		UpdatedAt: s.now().UTC(),
	}

	steps := []struct {
		name string
		fn   func(context.Context, string) error
	}{
		{"delete_daily_reports", s.repo.DeleteDailyReportsByUser},
		{"delete_rss_feeds", s.repo.DeleteRSSFeedByUser},
		{"delete_user", s.repo.DeleteUser},
	}

	allOK := true
	for _, step := range steps {
		cs := CleanupStep{Name: step.name, Status: CleanupStatusCompleted}
		if err := step.fn(ctx, userID); err != nil {
			cs.Status = CleanupStatusFailed
			cs.Error = err.Error()
			allOK = false
		}
		task.Steps = append(task.Steps, cs)
		if !allOK {
			break
		}
	}

	if allOK {
		task.Status = CleanupStatusCompleted
	} else {
		task.Status = CleanupStatusFailed
	}
	task.UpdatedAt = s.now().UTC()
	_ = s.repo.SaveCleanupTask(ctx, task)
	return task, nil
}

func (s *Service) RetryCleanup(ctx context.Context, taskID string) (CleanupTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return CleanupTask{}, ErrInvalidInput
	}
	task, err := s.repo.CleanupTaskByID(ctx, taskID)
	if err != nil {
		return CleanupTask{}, err
	}
	if task.Status != CleanupStatusFailed {
		return task, nil
	}

	task.Status = CleanupStatusInProgress
	task.UpdatedAt = s.now().UTC()

	allOK := true
	for i, step := range task.Steps {
		if step.Status != CleanupStatusFailed {
			continue
		}
		var stepErr error
		switch step.Name {
		case "delete_daily_reports":
			stepErr = s.repo.DeleteDailyReportsByUser(ctx, task.UserID)
		case "delete_rss_feeds":
			stepErr = s.repo.DeleteRSSFeedByUser(ctx, task.UserID)
		case "delete_user":
			stepErr = s.repo.DeleteUser(ctx, task.UserID)
		}
		if stepErr != nil {
			task.Steps[i].Error = stepErr.Error()
			allOK = false
			break
		}
		task.Steps[i].Status = CleanupStatusCompleted
		task.Steps[i].Error = ""
	}

	if allOK {
		task.Status = CleanupStatusCompleted
	} else {
		task.Status = CleanupStatusFailed
	}
	task.UpdatedAt = s.now().UTC()
	_ = s.repo.SaveCleanupTask(ctx, task)
	return task, nil
}

func (s *Service) CleanupStatus(ctx context.Context, taskID string) (CleanupTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return CleanupTask{}, ErrInvalidInput
	}
	return s.repo.CleanupTaskByID(ctx, taskID)
}

func (s *Service) RevokeSource(ctx context.Context, sourceID string) (RevokedSource, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return RevokedSource{}, ErrInvalidInput
	}
	if s.cfg.RevokeSourceFunc == nil {
		return RevokedSource{}, ErrNotFound
	}
	return s.cfg.RevokeSourceFunc(ctx, sourceID)
}

func (s *Service) ConfigStatus(ctx context.Context) ConfigStatus {
	components := map[string]ComponentCheck{
		"postgresql": s.pingStatus(ctx, s.cfg.PostgreSQLPing),
		"redis":      s.pingStatus(ctx, s.cfg.RedisPing),
		"dashscope":  configPresenceStatus(s.cfg.DashScopeKey),
		"smtp":       configPresenceStatus(s.cfg.SMTPHost),
	}
	overall := ComponentStatusOK
	for _, component := range components {
		if component.Status != ComponentStatusOK {
			overall = ComponentStatusDegraded
			break
		}
	}
	return ConfigStatus{Overall: overall, Components: components}
}

func (s *Service) pingStatus(ctx context.Context, ping func(context.Context) error) ComponentCheck {
	if ping == nil {
		return ComponentCheck{Status: ComponentStatusOK}
	}
	if err := ping(ctx); err != nil {
		return ComponentCheck{Status: ComponentStatusDegraded, Reason: "unavailable"}
	}
	return ComponentCheck{Status: ComponentStatusOK}
}

func configPresenceStatus(value string) ComponentCheck {
	if strings.TrimSpace(value) == "" {
		return ComponentCheck{Status: ComponentStatusDegraded, Reason: "missing_config"}
	}
	return ComponentCheck{Status: ComponentStatusOK}
}

func newID(prefix string) string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
