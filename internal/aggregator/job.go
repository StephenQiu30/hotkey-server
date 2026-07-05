package aggregator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"gorm.io/gorm"
)

// TopicProvider provides X Topics for aggregation.
type TopicProvider interface {
	GetRecentTopics(ctx context.Context, since time.Time) ([]TopicBrief, error)
}

// TopicBrief is a minimal Topic representation for matching.
type TopicBrief struct {
	ID       int64
	MonitorID int64
	Title    string
	Key      string
	Heat     float64
	SeenAt   time.Time
}

// TrendingPostProvider provides recent platform trending posts.
type TrendingPostProvider interface {
	GetRecentTrendingPosts(ctx context.Context, since time.Time) ([]TrendingPostBrief, error)
}

// TrendingPostBrief is a minimal representation of a trending platform post.
type TrendingPostBrief struct {
	ID        int64
	Platform  string
	Title     string
	Heat      float64
	SeenAt    time.Time
}

// HotEventAggregatorJob periodically merges X Topics and trending data into HotEvents.
type HotEventAggregatorJob struct {
	matcher    *EventMatcher
	topicProv  TopicProvider
	trendProv  TrendingPostProvider
	eventSvc   *hotevent.Service
}

// NewHotEventAggregatorJob creates a new aggregation job.
func NewHotEventAggregatorJob(
	topicProv TopicProvider,
	trendProv TrendingPostProvider,
	eventSvc *hotevent.Service,
) *HotEventAggregatorJob {
	return &HotEventAggregatorJob{
		matcher:   DefaultMatcher(),
		topicProv: topicProv,
		trendProv: trendProv,
		eventSvc: eventSvc,
	}
}

// Register registers the aggregation job with the runner.
func Register(runner *jobs.Runner, db *gorm.DB, eventSvc *hotevent.Service) {
	topicProv := NewTopicQueryRepo(db)
	trendProv := NewTrendingPostQueryRepo(db)
	job := NewHotEventAggregatorJob(topicProv, trendProv, eventSvc)

	runner.Register("aggregate_events", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "aggregate_events: running"))
		return job.Run(ctx)
	}, 5*time.Minute)
}

// Run executes one aggregation cycle.
func (j *HotEventAggregatorJob) Run(ctx context.Context) error {
	since := time.Now().Add(-24 * time.Hour)

	// 1. Fetch recent X Topics
	topics, err := j.topicProv.GetRecentTopics(ctx, since)
	if err != nil {
		return fmt.Errorf("get topics: %w", err)
	}

	// 2. Fetch recent trending posts
	trends, err := j.trendProv.GetRecentTrendingPosts(ctx, since)
	if err != nil {
		return fmt.Errorf("get trends: %w", err)
	}

	log.Printf("aggregate_events: %d topics, %d trending posts to process", len(topics), len(trends))

	// 3. For each trending post, try to match with existing HotEvents
	var matched int
	for _, trend := range trends {
		if err := j.matchAndCreate(ctx, trend, topics); err != nil {
			log.Printf("aggregate_events: match error for %q: %v", trend.Title, err)
			continue
		}
		matched++
	}

	log.Printf("aggregate_events: matched %d/%d trending items", matched, len(trends))
	return nil
}

func (j *HotEventAggregatorJob) matchAndCreate(ctx context.Context, trend TrendingPostBrief, topics []TopicBrief) error {
	repo := j.eventSvc.Repo()

	// Try matching with existing active HotEvents
	existing, _, err := repo.List(hotevent.ListFilter{Status: hotevent.StatusActive, Limit: 100})
	if err != nil {
		return err
	}

	for _, ev := range existing {
		result := j.matcher.Match(trend.Title, ev.Name, trend.SeenAt, ev.LastSeenAt)
		if result.IsMatch {
			// Update existing event
			ev.LastSeenAt = time.Now()
			ev.HeatScore = hotevent.ComputeHeatScore("multi", []float64{ev.HeatScore, trend.Heat}, time.Now())
			ev.PostIDs = append(ev.PostIDs, trend.ID)
			if ev.Platform == "" || ev.Platform == trend.Platform {
				ev.Platform = trend.Platform
			} else {
				ev.Platform = "multi"
			}
			ev.Trend = hotevent.DetermineTrend(ev.HeatScore, ev.HeatScore*0.9)
			if err := repo.Update(ev); err != nil {
				return fmt.Errorf("update event %d: %w", ev.ID, err)
			}

			// Add platform detail
			_ = repo.AddPlatform(ev.ID, &hotevent.EventPlatform{
				Platform: trend.Platform,
				Rank:     0,
				Title:    trend.Title,
				Heat:     trend.Heat,
			})
			return nil
		}
	}

	// Also try matching with X Topics (to link to a new HotEvent)
	var bestTopic *TopicBrief
	var bestScore float64
	for i := range topics {
		result := j.matcher.Match(trend.Title, topics[i].Title, trend.SeenAt, topics[i].SeenAt)
		if result.IsMatch && result.Score > bestScore {
			bestScore = result.Score
			bestTopic = &topics[i]
		}
	}

	// Create new HotEvent
	event := &hotevent.HotEvent{
		Name:        trend.Title,
		HeatScore:   trend.Heat,
		Platform:    trend.Platform,
		Trend:       hotevent.TrendRising,
		FirstSeenAt: trend.SeenAt,
		LastSeenAt:  time.Now(),
		PostIDs:     []int64{trend.ID},
		Status:      hotevent.StatusActive,
	}
	if bestTopic != nil {
		event.TopicIDs = []int64{bestTopic.ID}
		event.Platform = "multi"
		event.HeatScore = hotevent.ComputeHeatScore("multi", []float64{bestTopic.Heat, trend.Heat}, time.Now())
	}

	if err := repo.Create(event); err != nil {
		return fmt.Errorf("create event: %w", err)
	}

	_ = repo.AddPlatform(event.ID, &hotevent.EventPlatform{
		Platform: trend.Platform,
		Title:    trend.Title,
		Heat:     trend.Heat,
	})
	return nil
}
