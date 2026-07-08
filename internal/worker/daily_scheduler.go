package worker

import (
	"fmt"
	"time"
)

type DailyScheduleConfig struct {
	Time     string
	Timezone string
	Target   string
}

func ResolveTargetDate(now time.Time, cfg DailyScheduleConfig) (time.Time, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(loc)
	switch cfg.Target {
	case "", "yesterday":
		local = local.AddDate(0, 0, -1)
	case "today":
	default:
		return time.Time{}, fmt.Errorf("invalid daily digest target: %s", cfg.Target)
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc), nil
}

func RunKeyForDate(date time.Time) string {
	return "daily-obsidian-publish:" + date.Format("2006-01-02")
}

func ShouldRun(now time.Time, lastRunDate *time.Time, cfg DailyScheduleConfig) (bool, time.Time, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return false, time.Time{}, err
	}
	hour, minute, err := parseClock(cfg.Time)
	if err != nil {
		return false, time.Time{}, err
	}
	local := now.In(loc)
	due := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	target, err := ResolveTargetDate(now, cfg)
	if err != nil {
		return false, time.Time{}, err
	}
	if local.Before(due) {
		return false, target, nil
	}
	if lastRunDate != nil && lastRunDate.Format("2006-01-02") == target.Format("2006-01-02") {
		return false, target, nil
	}
	return true, target, nil
}

func parseClock(value string) (int, int, error) {
	if value == "" {
		value = "08:00"
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, err
	}
	return parsed.Hour(), parsed.Minute(), nil
}
