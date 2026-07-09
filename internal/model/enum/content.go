package enum

// Platform defines values for the platform/text field used across the app.
// The canonical set is from db/schema.sql default values and hot_event_platforms.
type Platform string

const (
	PlatformX     Platform = "x"
	PlatformMulti Platform = "multi"
)

// TrendDirection defines the trend direction for topics and hot events.
type TrendDirection string

const (
	TrendRising    TrendDirection = "rising"
	TrendStable    TrendDirection = "stable"
	TrendDeclining TrendDirection = "declining"
	TrendFlat      TrendDirection = "flat"
)

// Severity defines alert severity levels.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// AlertType defines the type of an alert record.
type AlertType string

const (
	AlertTypeThreshold AlertType = "threshold"
	AlertTypeTrend     AlertType = "trend"
)

// DeliveryStatus defines the delivery state for notifications and emails.
type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusSent      DeliveryStatus = "sent"
	DeliveryStatusFailed    DeliveryStatus = "failed"
)