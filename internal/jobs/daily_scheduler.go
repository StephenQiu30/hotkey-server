package jobs

import (
	"time"
)

// DailyScheduler gates job execution to once per day after a target time.
type DailyScheduler struct {
	targetTime string // "HH:MM" in CST
	timezone   string // IANA timezone, default "Asia/Shanghai"
}

// NewDailyScheduler creates a daily gate; targetTime is "HH:MM" in timezone.
func NewDailyScheduler(targetTime, timezone string) *DailyScheduler {
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	return &DailyScheduler{
		targetTime: targetTime,
		timezone:   timezone,
	}
}

// ShouldRun returns true when the current time >= targetTime and it hasn't run today.
// lastRunDate is a "2006-01-02" string, or "" if never run.
func (s *DailyScheduler) ShouldRun(now time.Time, lastRunDate string) bool {
	loc, err := time.LoadLocation(s.timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}

	local := now.In(loc)
	today := local.Format("2006-01-02")

	if lastRunDate == today {
		return false
	}

	parsed, err := time.Parse("15:04", s.targetTime)
	if err != nil {
		return false
	}

	targetH, targetM := parsed.Hour(), parsed.Minute()

	if local.Hour() > targetH {
		return true
	}
	if local.Hour() == targetH && local.Minute() >= targetM {
		return true
	}

	return false
}

// MarkRun returns the date string the caller should persist.
func (s *DailyScheduler) MarkRun(now time.Time) string {
	loc, err := time.LoadLocation(s.timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return now.In(loc).Format("2006-01-02")
}
