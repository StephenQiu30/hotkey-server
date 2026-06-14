// Package digest implements daily digest topic selection and export logic.
package digest

import "time"

// CST is the Asia/Shanghai timezone (UTC+8).
var CST = time.FixedZone("CST", 8*3600)

// Window represents a half-open time interval [Start, End).
type Window struct {
	Start time.Time
	End   time.Time
}

// Contains reports whether t is within the half-open window [Start, End).
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

// DayWindow returns the half-open UTC interval [D 00:00 CST, D+1 00:00 CST)
// for the given date interpreted in the CST timezone.
func DayWindow(date time.Time) Window {
	// Normalize to midnight CST of the given date.
	y, m, d := date.Date()
	startCST := time.Date(y, m, d, 0, 0, 0, 0, CST)
	endCST := startCST.AddDate(0, 0, 1)

	return Window{
		Start: startCST.UTC(),
		End:   endCST.UTC(),
	}
}

// ResolveExportDate interprets "today" or "yesterday" relative to now in CST
// and returns the corresponding date at midnight CST.
func ResolveExportDate(now time.Time, target string) time.Time {
	nowCST := now.In(CST)
	y, m, d := nowCST.Date()

	switch target {
	case "yesterday":
		d--
	}
	return time.Date(y, m, d, 0, 0, 0, 0, CST)
}
