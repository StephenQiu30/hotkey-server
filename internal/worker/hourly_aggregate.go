package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/service"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// HourlyAggregateDeps groups dependencies for the hourly aggregate job.
type HourlyAggregateDeps struct {
	DB             *gorm.DB
	CollectRepo    *repository.CollectRepo
	TopicWriteRepo *repository.TopicWriteRepo
	SnapshotRepo   *repository.SnapshotRepo
	RunRepo        RunRepository
	Now            func() time.Time
}

// HourlyAggregateJob runs topic clustering -> hot event aggregation -> snapshots hourly.
type HourlyAggregateJob struct {
	deps HourlyAggregateDeps
}

func NewHourlyAggregateJob(deps HourlyAggregateDeps) *HourlyAggregateJob {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &HourlyAggregateJob{deps: deps}
}

func (j *HourlyAggregateJob) Type() string { return "hourly.run" }

func (j *HourlyAggregateJob) Handle(ctx context.Context, msg queue.Message) error {
	var payload struct {
		TargetHour string `json:"target_hour,omitempty"`
	}
	_ = json.Unmarshal(msg.Payload, &payload)

	now := j.deps.Now()
	runKey := fmt.Sprintf("hourly-aggregate:%s", now.Format("2006-01-02T15:00"))
	log := logging.L().With(zap.String("run_key", runKey))

	started, err := j.deps.RunRepo.TryStart(ctx, runKey, "hourly-aggregate", now, now)
	if err != nil {
		return err
	}
	if !started {
		log.Info("hourly aggregate already running, skipping")
		return nil
	}

	runErr := j.executeAll(ctx, now)
	if runErr != nil {
		_ = j.deps.RunRepo.MarkFailed(ctx, runKey, runErr.Error(), j.deps.Now())
		log.Error("hourly aggregate failed", zap.Error(runErr))
		return runErr
	}
	_ = j.deps.RunRepo.MarkFinished(ctx, runKey, j.deps.Now())
	log.Info("hourly aggregate completed")
	return nil
}

func (j *HourlyAggregateJob) DedupeEnabled() bool { return false }

func (j *HourlyAggregateJob) executeAll(ctx context.Context, now time.Time) error {
	since := now.Add(-1 * time.Hour)

	if err := j.clusterPosts(ctx, since); err != nil {
		return fmt.Errorf("cluster: %w", err)
	}
	if err := j.aggregateHotEvents(ctx); err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}
	if err := j.snapshotTrends(ctx); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	return nil
}

func (j *HourlyAggregateJob) clusterPosts(ctx context.Context, since time.Time) error {
	hits, err := j.deps.CollectRepo.ListHitsSince(ctx, since)
	if err != nil {
		return fmt.Errorf("list hits: %w", err)
	}
	if len(hits) == 0 {
		return nil
	}

	postIDSet := make(map[int64]struct{})
	monitorIDByPost := make(map[int64]int64)
	for _, h := range hits {
		postIDSet[h.PostID] = struct{}{}
		monitorIDByPost[h.PostID] = h.MonitorID
	}
	postIDs := make([]int64, 0, len(postIDSet))
	for id := range postIDSet {
		postIDs = append(postIDs, id)
	}

	type postRecord struct {
		ID          int64
		ContentText string
	}
	var posts []postRecord
	if err := j.deps.DB.WithContext(ctx).
		Model(&entity.PlatformPost{}).
		Select("id, content_text").
		Where("id IN ?", postIDs).
		Find(&posts).Error; err != nil {
		return fmt.Errorf("load posts: %w", err)
	}

	candidates := make([]service.CandidatePost, len(posts))
	for i, p := range posts {
		candidates[i] = service.CandidatePost{
			PostID: p.ID,
			Tokens: service.ExtractTokens(p.ContentText),
		}
	}

	clustered := service.Cluster(candidates)
	logging.L().Info("topic clustering completed", zap.Int("clusters", len(clustered)))

	for _, c := range clustered {
		monitorID := int64(0)
		for _, pid := range c.PostIDs {
			if mid, ok := monitorIDByPost[pid]; ok {
				monitorID = mid
				break
			}
		}
		if monitorID == 0 {
			continue
		}
		topicID, err := j.deps.TopicWriteRepo.CreateTopic(ctx, monitorID, c.TopicKey, c.Title, "")
		if err != nil {
			logging.L().Error("failed to create topic", zap.String("key", c.TopicKey), zap.Error(err))
			continue
		}
		for _, pid := range c.PostIDs {
			if err := j.deps.TopicWriteRepo.AddTopicPost(ctx, topicID, pid, 1.0); err != nil {
				logging.L().Error("failed to add topic post",
					zap.Int64("topic_id", topicID),
					zap.Int64("post_id", pid),
					zap.Error(err),
				)
			}
		}
	}
	return nil
}

func (j *HourlyAggregateJob) aggregateHotEvents(ctx context.Context) error {
	type topicRecord struct {
		ID               int64
		MonitorID        int64
		Title            string
		CurrentHeatScore float64
	}
	var topics []topicRecord
	if err := j.deps.DB.WithContext(ctx).
		Model(&entity.Topic{}).
		Select("id, monitor_id, title, current_heat_score").
		Where("status = ?", "active").
		Find(&topics).Error; err != nil {
		return fmt.Errorf("list topics: %w", err)
	}

	for _, t := range topics {
		heat := service.ComputeHeatScore("x", []float64{t.CurrentHeatScore}, time.Now())
		direction := service.DetermineTrend(heat, t.CurrentHeatScore)

		event := entity.HotEvent{
			Name:        t.Title,
			HeatScore:   heat,
			Platform:    "x",
			Trend:       direction,
			FirstSeenAt: time.Now(),
			LastSeenAt:  time.Now(),
			Status:      service.StatusActive,
		}
		if err := j.deps.DB.WithContext(ctx).Create(&event).Error; err != nil {
			return fmt.Errorf("create hot event: %w", err)
		}
	}
	return nil
}

func (j *HourlyAggregateJob) snapshotTrends(ctx context.Context) error {
	now := j.deps.Now()

	type topicHeat struct {
		ID               int64
		CurrentHeatScore float64
	}
	var topicHeats []topicHeat
	if err := j.deps.DB.WithContext(ctx).
		Model(&entity.Topic{}).
		Select("id, current_heat_score").
		Where("status = ?", "active").
		Find(&topicHeats).Error; err != nil {
		return fmt.Errorf("list topics for snapshot: %w", err)
	}

	for _, th := range topicHeats {
		prev, err := j.deps.SnapshotRepo.GetTopicSnapshotBefore(ctx, th.ID, now)
		if err != nil {
			return fmt.Errorf("get prev snapshot topic %d: %w", th.ID, err)
		}
		prevHeat := 0.0
		if prev != nil {
			prevHeat = prev.HeatScore
		}
		snap := service.BuildTopicSnapshot(service.TopicSnapshotInput{
			TopicID:      th.ID,
			SnapshotTime: now,
			HeatScore:    th.CurrentHeatScore,
			PreviousHeat: prevHeat,
		})
		gormSnap := &entity.TopicSnapshot{
			TopicID:       snap.TopicID,
			SnapshotTime:  snap.SnapshotTime,
			HeatScore:     snap.HeatScore,
			TrendVelocity: snap.TrendVelocity,
		}
		if err := j.deps.SnapshotRepo.CreateTopicSnapshot(ctx, gormSnap); err != nil {
			return fmt.Errorf("create topic snapshot %d: %w", th.ID, err)
		}
	}

	var monitorIDs []int64
	if err := j.deps.DB.WithContext(ctx).
		Model(&entity.KeywordMonitor{}).
		Where("status = ?", "active").
		Pluck("id", &monitorIDs).Error; err != nil {
		return fmt.Errorf("list monitors for snapshot: %w", err)
	}
	for _, mid := range monitorIDs {
		snap := service.BuildMonitorSnapshot(service.MonitorSnapshotInput{
			MonitorID:   mid,
			SnapshotTime: now,
		})
		gormSnap := &entity.MonitorSnapshot{
			MonitorID:    snap.MonitorID,
			SnapshotTime: snap.SnapshotTime,
		}
		if err := j.deps.SnapshotRepo.CreateMonitorSnapshot(ctx, gormSnap); err != nil {
			return fmt.Errorf("create monitor snapshot %d: %w", mid, err)
		}
	}
	return nil
}
