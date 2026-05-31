package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type JobType string

const (
	JobTypeCollectSource       JobType = "collect_source"
	JobTypeGenerateEmbedding   JobType = "generate_embedding"
	JobTypeClusterHotspots     JobType = "cluster_hotspots"
	JobTypeScoreHotspots       JobType = "score_hotspots"
	JobTypeGenerateDailyReport JobType = "generate_daily_report"
	JobTypeSendDailyEmail      JobType = "send_daily_email"
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
	default:
		return fmt.Errorf("unknown job type %q", jobType)
	}
	return nil
}
