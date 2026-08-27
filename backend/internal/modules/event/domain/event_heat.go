package domain

import (
	"errors"
	"math"
	"strings"
)

type EventHeatInput struct {
	IndependentLineageRoots   int
	ReportsInWindow           int
	ReportsInPreviousWindow   int
	ReportsInPriorWindow      int
	PublisherCoverage         int
	SourceTypeCoverage        int
	NormalizedEngagement      *float64
	TemporalBaselineAvailable bool
	AgeHours                  float64
	ProfileVersion            string
	Weights                   EventHeatWeights
}

type EventHeatWeights struct {
	Lineage, Velocity, Acceleration, Coverage, Engagement, Recency float64
}

type EventHeatResult struct {
	Score                   float64
	IndependentLineageRoots int
	LineageBreadth          float64
	Velocity                float64
	Acceleration            float64
	Coverage                float64
	NormalizedEngagement    *float64
	Recency                 float64
	AvailableWeight         float64
	WarmingUp               bool
	ProfileVersion          string
	ReasonCodes             []string
}

func CalculateEventHeat(input EventHeatInput) (EventHeatResult, error) {
	if strings.TrimSpace(input.ProfileVersion) == "" || len(input.ProfileVersion) > 64 || input.IndependentLineageRoots < 0 ||
		input.ReportsInWindow < 0 || input.ReportsInPreviousWindow < 0 || input.ReportsInPriorWindow < 0 ||
		input.PublisherCoverage < 0 || input.SourceTypeCoverage < 0 || math.IsNaN(input.AgeHours) ||
		math.IsInf(input.AgeHours, 0) || input.AgeHours < 0 {
		return EventHeatResult{}, errors.New("invalid event heat input")
	}
	if input.NormalizedEngagement != nil && (math.IsNaN(*input.NormalizedEngagement) || math.IsInf(*input.NormalizedEngagement, 0) || *input.NormalizedEngagement < 0 || *input.NormalizedEngagement > 1) {
		return EventHeatResult{}, errors.New("invalid normalized engagement")
	}
	weights := input.Weights
	if weights == (EventHeatWeights{}) {
		weights = EventHeatWeights{Lineage: .25, Velocity: .20, Acceleration: .15, Coverage: .15, Engagement: .15, Recency: .10}
	}
	if !validEventHeatWeights(weights) {
		return EventHeatResult{}, errors.New("invalid event heat weights")
	}
	lineage := math.Min(1, float64(input.IndependentLineageRoots)/5)
	velocity, acceleration := 0.0, 0.0
	if input.TemporalBaselineAvailable {
		velocity = math.Min(1, float64(input.ReportsInWindow)/10)
		currentDelta := input.ReportsInWindow - input.ReportsInPreviousWindow
		previousDelta := input.ReportsInPreviousWindow - input.ReportsInPriorWindow
		acceleration = math.Min(1, math.Max(0, float64(currentDelta-previousDelta)/10))
	}
	coverage := math.Min(1, (float64(input.PublisherCoverage)/5+float64(input.SourceTypeCoverage)/4)/2)
	recency := math.Exp(-input.AgeHours / 24)
	weighted := lineage*weights.Lineage + coverage*weights.Coverage + recency*weights.Recency
	availableWeight := weights.Lineage + weights.Coverage + weights.Recency
	reasons := []string{}
	if input.TemporalBaselineAvailable {
		weighted += velocity*weights.Velocity + acceleration*weights.Acceleration
		availableWeight += weights.Velocity + weights.Acceleration
	} else {
		reasons = append(reasons, "warming_up")
	}
	if input.NormalizedEngagement != nil {
		weighted += *input.NormalizedEngagement * weights.Engagement
		availableWeight += weights.Engagement
	} else {
		reasons = append(reasons, "metrics_unavailable")
	}
	if availableWeight <= 0 {
		return EventHeatResult{}, errors.New("no available heat component")
	}
	engagement := input.NormalizedEngagement
	if engagement != nil {
		copy := *engagement
		engagement = &copy
	}
	return EventHeatResult{Score: weighted / availableWeight * 100, IndependentLineageRoots: input.IndependentLineageRoots,
		LineageBreadth: lineage, Velocity: velocity, Acceleration: acceleration, Coverage: coverage,
		NormalizedEngagement: engagement, Recency: recency, AvailableWeight: availableWeight,
		WarmingUp: !input.TemporalBaselineAvailable, ProfileVersion: input.ProfileVersion, ReasonCodes: reasons}, nil
}

func validEventHeatWeights(value EventHeatWeights) bool {
	weights := []float64{value.Lineage, value.Velocity, value.Acceleration, value.Coverage, value.Engagement, value.Recency}
	total := 0.0
	for _, weight := range weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 || weight > 1 {
			return false
		}
		total += weight
	}
	return math.Abs(total-1) <= 0.0000001
}
