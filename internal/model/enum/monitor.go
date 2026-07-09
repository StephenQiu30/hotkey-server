package enum

// MonitorStatus defines keyword monitor lifecycle states.
type MonitorStatus string

const (
	MonitorStatusActive   MonitorStatus = "active"
	MonitorStatusArchived MonitorStatus = "archived"
	MonitorStatusInactive MonitorStatus = "inactive"
)
