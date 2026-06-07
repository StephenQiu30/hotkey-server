package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

type JobType string

const (
	JobTypeCollectSource       JobType = "collect_source"
	JobTypeGenerateEmbedding   JobType = "generate_embedding"
	JobTypeClusterHotspots     JobType = "cluster_hotspots"
	JobTypeScoreHotspots       JobType = "score_hotspots"
	JobTypeGenerateDailyReport JobType = "generate_daily_report"
	JobTypeSendDailyEmail          JobType = "send_daily_email"
	JobTypeSendWeeklyEmail         JobType = "send_weekly_email"
	JobTypeGenerateWeeklyReport    JobType = "generate_weekly_report"
	JobTypeGenerateEventSummary    JobType = "generate_event_summary"
	JobTypeStoreSnapshot           JobType = "store_snapshot"
	JobTypeCleanupExpiredObjects   JobType = "cleanup_expired_objects"
	JobTypeDeleteUserObjects       JobType = "delete_user_objects"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusRunning    JobStatus = "running"
	JobStatusScheduled  JobStatus = "scheduled"
	JobStatusSucceeded  JobStatus = "succeeded"
	JobStatusFailed     JobStatus = "failed"
	JobStatusDeadLetter JobStatus = "dead_letter"
)

type Job struct {
	ID             string
	Type           JobType
	Payload        json.RawMessage
	Status         JobStatus
	Attempt        int
	MaxAttempts    int
	IdempotencyKey string
	LastError      string
	NextRunAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CollectSourcePayload struct {
	SourceID     string    `json:"source_id"`
	ScheduledFor time.Time `json:"scheduled_for"`
}

type GenerateEmbeddingPayload struct {
	ItemID string `json:"item_id"`
}

type ClusterHotspotsPayload struct {
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

type ScoreHotspotsPayload struct {
	ClusterRunID string `json:"cluster_run_id"`
}

type GenerateDailyReportPayload struct {
	Date string `json:"date"`
}

type SendDailyEmailPayload struct {
	ReportID        string `json:"report_id"`
	RecipientUserID string `json:"recipient_user_id"`
}

type SendWeeklyEmailPayload struct {
	ReportID        string `json:"report_id"`
	RecipientUserID string `json:"recipient_user_id"`
}

type GenerateWeeklyReportPayload struct {
	WeekOf string `json:"week_of"`
}

type EventSummaryItem struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
	URL      string `json:"url"`
}

type GenerateEventSummaryPayload struct {
	EventID string             `json:"event_id"`
	Title   string             `json:"title"`
	Items   []EventSummaryItem `json:"items"`
}

type StoreSnapshotPayload struct {
	SourceItemID string `json:"source_item_id"`
	SourceID     string `json:"source_id"`
	UserID       string `json:"user_id"`
	Platform     string `json:"platform"`
	Title        string `json:"title"`
	Snippet      string `json:"snippet"`
	OriginalURL  string `json:"original_url"`
}

type CleanupExpiredObjectsPayload struct {
	Bucket string `json:"bucket"`
}

type DeleteUserObjectsPayload struct {
	UserID string `json:"user_id"`
}

func ValidatePayload(jobType JobType, payload json.RawMessage) error {
	switch jobType {
	case JobTypeCollectSource:
		var body CollectSourcePayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.SourceID == "" || body.ScheduledFor.IsZero() {
			return errors.New("collect_source payload requires source_id and scheduled_for")
		}
	case JobTypeGenerateEmbedding:
		var body GenerateEmbeddingPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.ItemID == "" {
			return errors.New("generate_embedding payload requires item_id")
		}
	case JobTypeClusterHotspots:
		var body ClusterHotspotsPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.WindowStart.IsZero() || body.WindowEnd.IsZero() || !body.WindowEnd.After(body.WindowStart) {
			return errors.New("cluster_hotspots payload requires valid window_start and window_end")
		}
	case JobTypeScoreHotspots:
		var body ScoreHotspotsPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.ClusterRunID == "" {
			return errors.New("score_hotspots payload requires cluster_run_id")
		}
	case JobTypeGenerateDailyReport:
		var body GenerateDailyReportPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.Date == "" {
			return errors.New("generate_daily_report payload requires date")
		}
		if _, err := time.Parse("2006-01-02", body.Date); err != nil {
			return errors.New("generate_daily_report payload requires date in YYYY-MM-DD format")
		}
	case JobTypeSendDailyEmail:
		var body SendDailyEmailPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.ReportID == "" || body.RecipientUserID == "" {
			return errors.New("send_daily_email payload requires report_id and recipient_user_id")
		}
	case JobTypeSendWeeklyEmail:
		var body SendWeeklyEmailPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.ReportID == "" || body.RecipientUserID == "" {
			return errors.New("send_weekly_email payload requires report_id and recipient_user_id")
		}
	case JobTypeGenerateWeeklyReport:
		var body GenerateWeeklyReportPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.WeekOf == "" {
			return errors.New("generate_weekly_report payload requires week_of")
		}
		if matched, _ := regexp.MatchString(`^\d{4}-W(0[1-9]|[1-4]\d|5[0-3])$`, body.WeekOf); !matched {
			return errors.New("generate_weekly_report payload requires week_of in YYYY-Www format")
		}
	case JobTypeGenerateEventSummary:
		var body GenerateEventSummaryPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.EventID == "" {
			return errors.New("generate_event_summary payload requires event_id")
		}
		if body.Title == "" {
			return errors.New("generate_event_summary payload requires title")
		}
	case JobTypeStoreSnapshot:
		var body StoreSnapshotPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.SourceItemID == "" || body.SourceID == "" {
			return errors.New("store_snapshot payload requires source_item_id and source_id")
		}
	case JobTypeCleanupExpiredObjects:
		var body CleanupExpiredObjectsPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		// Empty Bucket is allowed — handler uses configured default
	case JobTypeDeleteUserObjects:
		var body DeleteUserObjectsPayload
		if err := json.Unmarshal(payload, &body); err != nil {
			return err
		}
		if body.UserID == "" {
			return errors.New("delete_user_objects payload requires user_id")
		}
	default:
		return fmt.Errorf("unknown job type %q", jobType)
	}
	return nil
}
