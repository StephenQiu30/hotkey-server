package jobs

import (
	"time"
)

// DailyScheduler gates job execution to once per day after a target CST time.
// It prevents duplicate runs on the same calendar day.
type DailyScheduler struct {
	targetTime string // "HH:MM" in CST
	timezone   string // IANA timezone, default "Asia/Shanghai"
}

// NewDailyScheduler creates a scheduler that gates execution.
// targetTime is "HH:MM" in the given timezone.
func NewDailyScheduler(targetTime, timezone string) *DailyScheduler {
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	return &DailyScheduler{
		targetTime: targetTime,
		timezone:   timezone,
	}
}

// ShouldRun reports whether the job should execute now.
// It returns true only when:
//  1. Current time in the configured timezone is >= targetTime
//  2. The job has not already run for today's date in that timezone
//
// lastRunDate should be the "2006-01-02" string of the most recent successful
// run, or "" if never run. The caller is responsible for persisting this value.
func (s *DailyScheduler) ShouldRun(now time.Time, lastRunDate string) bool {
	loc, err := time.LoadLocation(s.timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}

	local := now.In(loc)
	today := local.Format("2006-01-02")

	// Already ran today
	if lastRunDate == today {
		return false
	}

	// Parse target hour and minute
	parsed, err := time.Parse("15:04", s.targetTime)
	if err != nil {
		return false
	}

	targetH, targetM := parsed.Hour(), parsed.Minute()

	// Check if current time is past the target
	if local.Hour() > targetH {
		return true
	}
	if local.Hour() == targetH && local.Minute() >= targetM {
		return true
	}

	return false
}

// MarkRun records that a run completed for the given date.
// Returns the date string to be persisted by the caller.
func (s *DailyScheduler) MarkRun(now time.Time) string {
	loc, err := time.LoadLocation(s.timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return now.In(loc).Format("2006-01-02")
}
