package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

// ReportSubscription is the scheduler-safe projection of a persisted delivery
// subscription. It contains no recipient, user identity or token material.
type ReportSubscription struct {
	ID, Version int64
	ReportType  string
	Timezone    string
	Schedule    string
	Enabled     bool
}

type ReportSubscriptionDueReader interface {
	ListEnabledReportSubscriptions(context.Context) ([]ReportSubscription, error)
}

type ReportScheduler struct {
	reader ReportSubscriptionDueReader
	store  Enqueuer
}

func NewReportScheduler(reader ReportSubscriptionDueReader, store Enqueuer) *ReportScheduler {
	return &ReportScheduler{reader: reader, store: store}
}

// RunOnce enqueues at most one report job per subscription and local minute.
// The queue unique key makes repeated scheduler scans idempotent.
func (scheduler *ReportScheduler) RunOnce(ctx context.Context, now time.Time) (int, error) {
	if scheduler == nil || scheduler.reader == nil || scheduler.store == nil || now.IsZero() {
		return 0, fmt.Errorf("report scheduler is not initialized")
	}
	subscriptions, err := scheduler.reader.ListEnabledReportSubscriptions(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, subscription := range subscriptions {
		if subscription.ID <= 0 || subscription.Version <= 0 || !subscription.Enabled {
			continue
		}
		location, err := time.LoadLocation(defaultTimezone(subscription.Timezone))
		if err != nil {
			return created, fmt.Errorf("subscription %d timezone: %w", subscription.ID, err)
		}
		local := now.In(location).Truncate(time.Minute)
		if !CronMatches(subscription.Schedule, local) {
			continue
		}
		minute := local.UTC()
		hash := queue.StableJobHash(queue.KindBuildReport, fmt.Sprint(subscription.ID), fmt.Sprint(subscription.Version), minute.Format(time.RFC3339))
		_, wasCreated, err := scheduler.store.Enqueue(ctx, queue.Job{
			Kind:        queue.KindBuildReport,
			UniqueKey:   queue.StableJobKey(queue.KindBuildReport, subscription.ID, subscription.Version, hash),
			Payload:     queue.Payload{EntityID: subscription.ID, EntityVersion: subscription.Version, WindowStart: minute, InputHash: hash},
			ScheduledAt: now.UTC(), MaxAttempts: 5, Priority: 7,
		})
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}

func (scheduler *ReportScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("report scheduler interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := scheduler.RunOnce(ctx, time.Now().UTC()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// CronMatches supports the five-field schedules exposed by the delivery API:
// minute hour day-of-month month day-of-week. It covers *, */n, lists and
// ranges without introducing a third-party scheduler dependency.
func CronMatches(schedule string, at time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(schedule))
	if len(fields) != 5 {
		return false
	}
	return cronFieldMatches(fields[0], at.Minute(), 0, 59) &&
		cronFieldMatches(fields[1], at.Hour(), 0, 23) &&
		cronFieldMatches(fields[2], at.Day(), 1, 31) &&
		cronFieldMatches(fields[3], int(at.Month()), 1, 12) &&
		cronWeekdayMatches(fields[4], int(at.Weekday()))
}

func cronWeekdayMatches(expression string, weekday int) bool {
	return cronFieldMatches(expression, weekday, 0, 7) || weekday == 0 && cronFieldMatches(expression, 7, 0, 7)
}

func cronFieldMatches(expression string, value, minimum, maximum int) bool {
	for _, part := range strings.Split(strings.TrimSpace(expression), ",") {
		if part == "*" {
			return true
		}
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(strings.TrimPrefix(part, "*/"))
			if err == nil && step > 0 && (value-minimum)%step == 0 {
				return true
			}
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, startErr := strconv.Atoi(bounds[0])
			end, endErr := strconv.Atoi(bounds[1])
			if startErr == nil && endErr == nil && start >= minimum && end <= maximum && start <= value && value <= end {
				return true
			}
			continue
		}
		candidate, err := strconv.Atoi(part)
		if err == nil && candidate >= minimum && candidate <= maximum && candidate == value {
			return true
		}
	}
	return false
}

func defaultTimezone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "UTC"
	}
	return strings.TrimSpace(value)
}
