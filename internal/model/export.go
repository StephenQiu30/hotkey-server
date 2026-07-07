package model

import "time"

// ExportBundle is a batch export of monitor data.
type ExportBundle struct {
	ID        int64
	MonitorID int64
	BundleKey string
	BundleKind string
	DateStart *time.Time
	DateEnd   *time.Time
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
