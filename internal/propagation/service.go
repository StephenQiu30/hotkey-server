package propagation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LayerFact   = "fact"
	LayerSignal = "signal"

	StatusNoClaims  = "no_claims"
	StatusConfirmed = "confirmed"
	StatusConflict  = "conflict"
)

var (
	ErrInvalidStep  = errors.New("invalid propagation step")
	ErrInvalidClaim = errors.New("invalid arbitration claim")
)

type StepInput struct {
	EventID    string    `json:"eventId"`
	SourceID   string    `json:"sourceId"`
	Layer      string    `json:"layer"`
	URL        string    `json:"url"`
	ObservedAt time.Time `json:"observedAt"`
	Note       string    `json:"note"`
}

type Step struct {
	EventID    string    `json:"eventId"`
	SourceID   string    `json:"sourceId"`
	Layer      string    `json:"layer"`
	URL        string    `json:"url"`
	ObservedAt time.Time `json:"observedAt"`
	Note       string    `json:"note,omitempty"`
}

type Path struct {
	EventID string `json:"eventId"`
	Steps   []Step `json:"steps"`
}

type ClaimInput struct {
	EventID    string `json:"eventId"`
	ClaimKey   string `json:"claimKey"`
	Value      string `json:"value"`
	SourceID   string `json:"sourceId"`
	Layer      string `json:"layer"`
	TrustScore int    `json:"trustScore"`
}

type Claim struct {
	EventID    string `json:"eventId"`
	ClaimKey   string `json:"claimKey"`
	Value      string `json:"value"`
	SourceID   string `json:"sourceId"`
	Layer      string `json:"layer"`
	TrustScore int    `json:"trustScore"`
}

type Conflict struct {
	ClaimKey string   `json:"claimKey"`
	Values   []string `json:"values"`
	Sources  []string `json:"sources"`
}

type ArbitrationResult struct {
	EventID       string     `json:"eventId"`
	Status        string     `json:"status"`
	WinningValue  string     `json:"winningValue,omitempty"`
	WinningSource string     `json:"winningSource,omitempty"`
	Explanation   string     `json:"explanation"`
	Conflicts     []Conflict `json:"conflicts,omitempty"`
	Claims        []Claim    `json:"claims"`
}

type Service struct {
	mu     sync.Mutex
	steps  map[string][]Step
	claims map[string][]Claim
}

func NewService() *Service {
	return &Service{
		steps:  make(map[string][]Step),
		claims: make(map[string][]Claim),
	}
}

func (s *Service) AddStep(input StepInput) error {
	step, err := normalizeStep(input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps[step.EventID] = append(s.steps[step.EventID], step)
	return nil
}

func (s *Service) GetPath(eventID string) Path {
	eventID = strings.TrimSpace(eventID)
	s.mu.Lock()
	defer s.mu.Unlock()

	steps := append([]Step(nil), s.steps[eventID]...)
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].ObservedAt.Before(steps[j].ObservedAt)
	})
	return Path{EventID: eventID, Steps: steps}
}

func (s *Service) AddClaim(input ClaimInput) error {
	claim, err := normalizeClaim(input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims[claim.EventID] = append(s.claims[claim.EventID], claim)
	return nil
}

func (s *Service) Arbitrate(eventID string) ArbitrationResult {
	eventID = strings.TrimSpace(eventID)
	s.mu.Lock()
	defer s.mu.Unlock()

	claims := append([]Claim(nil), s.claims[eventID]...)
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].ClaimKey == claims[j].ClaimKey {
			return claims[i].SourceID < claims[j].SourceID
		}
		return claims[i].ClaimKey < claims[j].ClaimKey
	})
	if len(claims) == 0 {
		return ArbitrationResult{
			EventID:     eventID,
			Status:      StatusNoClaims,
			Explanation: "no source claims recorded",
		}
	}

	result := ArbitrationResult{
		EventID: eventID,
		Status:  StatusConfirmed,
		Claims:  claims,
	}
	bestClaim := claims[0]
	for _, claim := range claims {
		if claim.TrustScore > bestClaim.TrustScore {
			bestClaim = claim
		}
	}
	result.WinningValue = bestClaim.Value
	result.WinningSource = bestClaim.SourceID
	result.Explanation = fmt.Sprintf("highest trust source %s selected", bestClaim.SourceID)

	for _, conflict := range detectFactConflicts(claims) {
		result.Status = StatusConflict
		result.Conflicts = append(result.Conflicts, conflict)
	}
	if result.Status == StatusConflict {
		result.Explanation = fmt.Sprintf("conflicting fact sources for %s; highest trust value %s selected", result.Conflicts[0].ClaimKey, result.WinningValue)
	}
	return result
}

func normalizeStep(input StepInput) (Step, error) {
	step := Step{
		EventID:    strings.TrimSpace(input.EventID),
		SourceID:   strings.TrimSpace(input.SourceID),
		Layer:      strings.TrimSpace(input.Layer),
		URL:        strings.TrimSpace(input.URL),
		ObservedAt: input.ObservedAt.UTC(),
		Note:       strings.TrimSpace(input.Note),
	}
	if step.EventID == "" || step.SourceID == "" || step.URL == "" || !validLayer(step.Layer) || step.ObservedAt.IsZero() {
		return Step{}, ErrInvalidStep
	}
	return step, nil
}

func normalizeClaim(input ClaimInput) (Claim, error) {
	claim := Claim{
		EventID:    strings.TrimSpace(input.EventID),
		ClaimKey:   strings.TrimSpace(input.ClaimKey),
		Value:      strings.TrimSpace(input.Value),
		SourceID:   strings.TrimSpace(input.SourceID),
		Layer:      strings.TrimSpace(input.Layer),
		TrustScore: input.TrustScore,
	}
	if claim.EventID == "" || claim.ClaimKey == "" || claim.Value == "" || claim.SourceID == "" || !validLayer(claim.Layer) || claim.TrustScore < 0 {
		return Claim{}, ErrInvalidClaim
	}
	return claim, nil
}

func validLayer(layer string) bool {
	return layer == LayerFact || layer == LayerSignal
}

func detectFactConflicts(claims []Claim) []Conflict {
	byKey := make(map[string][]Claim)
	for _, claim := range claims {
		if claim.Layer != LayerFact {
			continue
		}
		byKey[claim.ClaimKey] = append(byKey[claim.ClaimKey], claim)
	}

	conflicts := make([]Conflict, 0)
	for key, factClaims := range byKey {
		values := make(map[string]bool)
		sources := make(map[string]bool)
		for _, claim := range factClaims {
			values[claim.Value] = true
			sources[claim.SourceID] = true
		}
		if len(values) <= 1 {
			continue
		}
		conflict := Conflict{ClaimKey: key}
		for value := range values {
			conflict.Values = append(conflict.Values, value)
		}
		for source := range sources {
			conflict.Sources = append(conflict.Sources, source)
		}
		sort.Strings(conflict.Values)
		sort.Strings(conflict.Sources)
		conflicts = append(conflicts, conflict)
	}
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].ClaimKey < conflicts[j].ClaimKey
	})
	return conflicts
}
