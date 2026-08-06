package application

import "context"

// PublishedAlertPolicy contains only the active, published Monitor facts that
// Alert needs to evaluate and freeze. Draft rules and Source configuration do
// not cross this boundary.
type PublishedAlertPolicy struct {
	MonitorID       int64
	ConfigVersionID int64
	Revision        int64
	ConfigHash      string
	EventThreshold  float64
}

type AlertPolicyReader interface {
	ListPublishedAlertPolicies(context.Context, []int64) ([]PublishedAlertPolicy, error)
}
