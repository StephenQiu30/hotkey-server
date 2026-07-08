package hotevent

import (
	"math"
	"time"
)

// Status constants for HotEvent lifecycle.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Trend direction constants.
const (
	TrendRising    = "rising"
	TrendStable    = "stable"
	TrendDeclining = "declining"
)

// PlatformWeights defines the relative weight of each platform.
var PlatformWeights = map[string]float64{
	"x":     1.0,
	"weibo": 1.0,
	"zhihu": 0.8,
	"baidu": 0.7,
	"multi": 1.0,
}

// ComputeHeatScore calculates the composite heat score for a HotEvent.
func ComputeHeatScore(platform string, heats []float64, lastSeen time.Time) float64 {
	w := PlatformWeights[platform]
	if w == 0 {
		w = 0.5
	}

	hoursSinceUpdate := time.Since(lastSeen).Hours()
	decay := math.Exp(-0.01 * hoursSinceUpdate)

	var sum float64
	for _, h := range heats {
		sum += h * decay
	}

	return math.Round(w*sum*100) / 100
}

// DetermineTrend compares current heat to previous heat.
func DetermineTrend(current, previous float64) string {
	if current > previous*1.1 {
		return TrendRising
	}
	if current < previous*0.9 {
		return TrendDeclining
	}
	return TrendStable
}
