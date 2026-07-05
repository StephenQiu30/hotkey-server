// Package digest implements daily digest topic selection and export.
package digest

import "time"

// CST is the Asia/Shanghai timezone (UTC+8).
var CST = time.FixedZone("CST", 8*3600)

type Window struct {
	Start time.Time
	End   time.Time
}

func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

// DayWindow returns [D 00:00 CST, D+1 00:00 CST) for the given date.
func DayWindow(date time.Time) Window {
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
