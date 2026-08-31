package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	RadarCursorVersionV1     = 1
	RadarCursorMaximumEvents = 100
)

type RadarWindow string

const (
	RadarWindow1Hour   RadarWindow = "1h"
	RadarWindow6Hours  RadarWindow = "6h"
	RadarWindow24Hours RadarWindow = "24h"
	RadarWindow7Days   RadarWindow = "7d"
)

func (window RadarWindow) Valid() bool {
	switch window {
	case RadarWindow1Hour, RadarWindow6Hours, RadarWindow24Hours, RadarWindow7Days:
		return true
	default:
		return false
	}
}

func (window RadarWindow) Duration() time.Duration {
	switch window {
	case RadarWindow1Hour:
		return time.Hour
	case RadarWindow6Hours:
		return 6 * time.Hour
	case RadarWindow24Hours:
		return 24 * time.Hour
	case RadarWindow7Days:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

type RadarSort string

const (
	RadarSortMomentum  RadarSort = "momentum"
	RadarSortAttention RadarSort = "attention"
	RadarSortBreadth   RadarSort = "breadth"
	RadarSortLatest    RadarSort = "latest"
	RadarSortRelevance RadarSort = "relevance"
)

func (sortValue RadarSort) Valid() bool {
	switch sortValue {
	case RadarSortMomentum, RadarSortAttention, RadarSortBreadth, RadarSortLatest, RadarSortRelevance:
		return true
	default:
		return false
	}
}

type RadarConfirmation string

const (
	RadarConfirmationDisputed     RadarConfirmation = "disputed"
	RadarConfirmationCorroborated RadarConfirmation = "corroborated"
	RadarConfirmationSingleSource RadarConfirmation = "single_source"
	RadarConfirmationUnverified   RadarConfirmation = "unverified"
	RadarConfirmationInsufficient RadarConfirmation = "insufficient"
)

func (confirmation RadarConfirmation) Valid() bool {
	switch confirmation {
	case RadarConfirmationDisputed, RadarConfirmationCorroborated, RadarConfirmationSingleSource, RadarConfirmationUnverified, RadarConfirmationInsufficient:
		return true
	default:
		return false
	}
}

type RadarConfirmationResult struct {
	Status RadarConfirmation
	Score  *float64
}

func DeriveRadarConfirmation(statuses []ClaimStatus) (RadarConfirmationResult, error) {
	present := make(map[ClaimStatus]bool, len(statuses))
	for _, status := range statuses {
		switch status {
		case ClaimDisputed, ClaimCorroborated, ClaimSingleSource, ClaimUnverified:
			present[status] = true
		case ClaimRetracted:
		default:
			return RadarConfirmationResult{}, fmt.Errorf("invalid claim status %q", status)
		}
	}
	for _, candidate := range []struct {
		claim        ClaimStatus
		confirmation RadarConfirmation
		score        float64
	}{
		{ClaimDisputed, RadarConfirmationDisputed, 20},
		{ClaimCorroborated, RadarConfirmationCorroborated, 100},
		{ClaimSingleSource, RadarConfirmationSingleSource, 60},
		{ClaimUnverified, RadarConfirmationUnverified, 30},
	} {
		if present[candidate.claim] {
			score := candidate.score
			return RadarConfirmationResult{Status: candidate.confirmation, Score: &score}, nil
		}
	}
	return RadarConfirmationResult{Status: RadarConfirmationInsufficient}, nil
}

type RadarDimensionInput struct {
	HeatScore              float64
	TrendScore             float64
	IndependentSourceCount int
	LastSeenAt             time.Time
	AsOf                   time.Time
}

type RadarDimensions struct {
	Attention      float64
	Momentum       float64
	Breadth        float64
	Freshness      float64
	DataConfidence float64
}

func CalculateRadarDimensions(input RadarDimensionInput) (RadarDimensions, error) {
	if invalidRadarNumber(input.HeatScore) || invalidRadarNumber(input.TrendScore) || input.IndependentSourceCount < 0 || input.LastSeenAt.IsZero() || input.AsOf.IsZero() {
		return RadarDimensions{}, fmt.Errorf("invalid Radar dimension input")
	}
	ageHours := math.Max(0, input.AsOf.Sub(input.LastSeenAt).Hours())
	attention := clampRadarScore(input.HeatScore)
	momentum := clampRadarScore((input.TrendScore + 100) / 2)
	breadth := clampRadarScore(float64(input.IndependentSourceCount) * 25)
	freshness := clampRadarScore(100 * math.Exp(-math.Ln2*ageHours/24))
	confidence := clampRadarScore(breadth*0.7 + freshness*0.3)
	return RadarDimensions{
		Attention: roundRadarScore(attention), Momentum: roundRadarScore(momentum),
		Breadth: roundRadarScore(breadth), Freshness: roundRadarScore(freshness),
		DataConfidence: roundRadarScore(confidence),
	}, nil
}

func RadarRankingScore(sortValue RadarSort, dimensions RadarDimensions, monitorFinalScore *float64) (float64, error) {
	var score float64
	switch sortValue {
	case RadarSortMomentum:
		score = dimensions.Momentum
	case RadarSortAttention:
		score = dimensions.Attention
	case RadarSortBreadth:
		score = dimensions.Breadth
	case RadarSortLatest:
		score = dimensions.Freshness
	case RadarSortRelevance:
		if monitorFinalScore == nil || invalidRadarNumber(*monitorFinalScore) {
			return 0, fmt.Errorf("relevance ranking requires monitor final score")
		}
		score = *monitorFinalScore
	default:
		return 0, fmt.Errorf("invalid Radar sort")
	}
	if invalidRadarNumber(score) {
		return 0, fmt.Errorf("invalid Radar ranking score")
	}
	return roundRadarScore(clampRadarScore(score)), nil
}

type RadarQuery struct {
	Window        RadarWindow
	Keyword       string
	MonitorID     *int64
	Lifecycles    []LifecycleStatus
	Trends        []TrendStatus
	Verifications []RadarConfirmation
	MinHeat       *float64
	Sort          RadarSort
	Limit         int
	Cursor        string
	AsOf          time.Time
}

func (query RadarQuery) Validate() error {
	if !query.Window.Valid() || !query.Sort.Valid() || query.Limit < 1 || query.Limit > 100 || query.AsOf.IsZero() || len([]rune(strings.TrimSpace(query.Keyword))) > 100 {
		return fmt.Errorf("invalid Radar query")
	}
	if query.MonitorID != nil && *query.MonitorID <= 0 || query.Sort == RadarSortRelevance && query.MonitorID == nil {
		return fmt.Errorf("invalid Radar monitor query")
	}
	if query.MinHeat != nil && (invalidRadarNumber(*query.MinHeat) || *query.MinHeat < 0 || *query.MinHeat > 100) {
		return fmt.Errorf("invalid Radar minimum heat")
	}
	for _, status := range query.Lifecycles {
		if !status.Valid() {
			return fmt.Errorf("invalid Radar lifecycle")
		}
	}
	for _, status := range query.Trends {
		if !status.Valid() {
			return fmt.Errorf("invalid Radar trend")
		}
	}
	for _, status := range query.Verifications {
		if !status.Valid() {
			return fmt.Errorf("invalid Radar verification")
		}
	}
	return nil
}

func (query RadarQuery) ShapeHash() (string, error) {
	if err := query.Validate(); err != nil {
		return "", err
	}
	monitor := ""
	if query.MonitorID != nil {
		monitor = strconv.FormatInt(*query.MonitorID, 10)
	}
	minimumHeat := ""
	if query.MinHeat != nil {
		minimumHeat = strconv.FormatFloat(*query.MinHeat, 'g', -1, 64)
	}
	parts := []string{
		"radar-query-v1", string(query.Window), strings.ToLower(strings.TrimSpace(query.Keyword)), monitor,
		sortedRadarValues(query.Lifecycles), sortedRadarValues(query.Trends), sortedRadarValues(query.Verifications),
		minimumHeat, string(query.Sort),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

type RadarCursor struct {
	Version      int                   `json:"v"`
	AsOf         time.Time             `json:"as_of"`
	ExpiresAt    time.Time             `json:"expires_at"`
	ShapeHash    string                `json:"shape"`
	RankingScore float64               `json:"score"`
	LastSeenAt   time.Time             `json:"last_seen_at"`
	EventID      int64                 `json:"event_id"`
	Remaining    []RadarCursorPosition `json:"remaining"`
}

type RadarCursorPosition struct {
	EventID      int64     `json:"event_id"`
	RankingScore float64   `json:"score"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

func (cursor RadarCursor) ValidateFor(query RadarQuery) error {
	return cursor.ValidateForAt(query, time.Now().UTC())
}

func (cursor RadarCursor) ValidateForAt(query RadarQuery, now time.Time) error {
	if cursor.Version != RadarCursorVersionV1 || cursor.AsOf.IsZero() || cursor.ExpiresAt.IsZero() || now.IsZero() || !cursor.ExpiresAt.After(now) || cursor.LastSeenAt.IsZero() || cursor.LastSeenAt.After(cursor.AsOf) || cursor.EventID <= 0 || len(cursor.ShapeHash) != 64 || invalidRadarNumber(cursor.RankingScore) || cursor.RankingScore < 0 || cursor.RankingScore > 100 || len(cursor.Remaining) == 0 || len(cursor.Remaining) >= RadarCursorMaximumEvents {
		return fmt.Errorf("invalid Radar cursor")
	}
	if _, err := hex.DecodeString(cursor.ShapeHash); err != nil {
		return fmt.Errorf("invalid Radar cursor")
	}
	want, err := query.ShapeHash()
	if err != nil {
		return err
	}
	if cursor.ShapeHash != want {
		return fmt.Errorf("radar cursor does not match query")
	}
	seen := map[int64]struct{}{cursor.EventID: {}}
	previous := RadarCursorPosition{EventID: cursor.EventID, RankingScore: cursor.RankingScore, LastSeenAt: cursor.LastSeenAt}
	for _, position := range cursor.Remaining {
		if position.EventID <= 0 || position.LastSeenAt.IsZero() || position.LastSeenAt.After(cursor.AsOf) || invalidRadarNumber(position.RankingScore) || position.RankingScore < 0 || position.RankingScore > 100 || !radarPositionFollows(previous, position) {
			return fmt.Errorf("invalid Radar cursor")
		}
		if _, exists := seen[position.EventID]; exists {
			return fmt.Errorf("invalid Radar cursor")
		}
		seen[position.EventID] = struct{}{}
		previous = position
	}
	return nil
}

func radarPositionFollows(previous, next RadarCursorPosition) bool {
	if next.RankingScore != previous.RankingScore {
		return next.RankingScore < previous.RankingScore
	}
	if !next.LastSeenAt.Equal(previous.LastSeenAt) {
		return next.LastSeenAt.Before(previous.LastSeenAt)
	}
	return next.EventID < previous.EventID
}

type RadarEvent struct {
	EventID                int64
	Version                int64
	EventKey               string
	TitleZH                string
	TitleEN                string
	Summary                string
	LifecycleStatus        LifecycleStatus
	FirstSeenAt            time.Time
	LastSeenAt             time.Time
	TrendScore             float64
	TrendStatus            TrendStatus
	Attention              float64
	Momentum               float64
	Breadth                float64
	IndependentSourceCount int
	Confirmation           RadarConfirmation
	ConfirmationScore      *float64
	DataConfidence         float64
	WatchRelevance         *float64
	WatchFinalScore        *float64
	RankingScore           float64
	ReasonCodes            []string
	LatestUpdate           *EventUpdate
}

type RadarPage struct {
	Items      []RadarEvent
	NextCursor string
	AsOf       time.Time
}

func sortedRadarValues[T ~string](values []T) string {
	items := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		text := string(value)
		if _, exists := seen[text]; exists {
			continue
		}
		seen[text] = struct{}{}
		items = append(items, text)
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func invalidRadarNumber(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) }
func clampRadarScore(value float64) float64 { return math.Max(0, math.Min(100, value)) }
func roundRadarScore(value float64) float64 { return math.Round(value*100) / 100 }
