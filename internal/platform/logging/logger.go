package logging

import (
	"encoding/json"
	"io"
	"time"
)

// Logger writes structured log events.
type Logger struct {
	out io.Writer
}

// Event is the common structured log field set for application events.
type Event struct {
	RequestID  string
	TraceID    string
	UserID     int64
	Module     string
	Action     string
	Err        error
	DurationMS int64
	Time       time.Time
}

// New creates a structured logger writing JSON lines to out.
func New(out io.Writer) *Logger {
	return &Logger{out: out}
}

// Event writes a structured event as one JSON line.
func (l *Logger) Event(event Event) {
	timestamp := event.Time
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	body := map[string]any{
		"time":        timestamp.UTC().Format(time.RFC3339),
		"request_id":  event.RequestID,
		"trace_id":    event.TraceID,
		"user_id":     event.UserID,
		"module":      event.Module,
		"action":      event.Action,
		"duration_ms": event.DurationMS,
	}
	if event.Err != nil {
		body["error"] = event.Err.Error()
	}

	_ = json.NewEncoder(l.out).Encode(body)
}
