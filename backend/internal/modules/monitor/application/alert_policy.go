package application

import "context"

// PublishedAlertPolicy contains only the active, published Monitor facts that
// Alert needs to evaluate and freeze. Draft rules and Source configuration do
// not cross this boundary.
type PublishedAlertPolicy struct {
	MonitorID              int64
	ConfigVersionID        int64
	Revision               int64
	ConfigHash             string
	EventThreshold         float64
	AlertMinHeat           float64
	AlertMinMomentum       float64
	AlertMinBreadth        float64
	AlertWarningThreshold  float64
	AlertCriticalThreshold float64
	AlertCooldownMinutes   int
	AlertEmailEnabled      bool
	AlertEmailMinSeverity  string
	RecipientEmail         string
}

type AlertPolicyReader interface {
	ListPublishedAlertPolicies(context.Context, []int64) ([]PublishedAlertPolicy, error)
}
