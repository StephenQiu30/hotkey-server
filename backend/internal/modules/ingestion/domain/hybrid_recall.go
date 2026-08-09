package domain

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var ErrInvalidRecallSignal = errors.New("hybrid recall signal is invalid")

// RecallChannel identifies one independently ranked retrieval channel. Raw
// scores are meaningful only inside their channel and are never fused.
type RecallChannel string

const (
	RecallChannelLexical    RecallChannel = "lexical"
	RecallChannelSemantic   RecallChannel = "semantic"
	RecallChannelStructured RecallChannel = "structured"
)

func (channel RecallChannel) Valid() bool {
	return channel == RecallChannelLexical || channel == RecallChannelSemantic || channel == RecallChannelStructured
}

// RecallSignal is the immutable provenance of one document's place in one
// ranked channel. The raw score is retained for explanation, never treated as
// a probability and never compared across channels.
type RecallSignal struct {
	documentVersionID int64
	channel           RecallChannel
	rank              int
	rawScore          float64
	algorithmVersion  string
}

func NewRecallSignal(documentVersionID int64, channel RecallChannel, rank int, rawScore float64, algorithmVersion string) (RecallSignal, error) {
	algorithmVersion = strings.TrimSpace(algorithmVersion)
	if documentVersionID <= 0 || !channel.Valid() || rank <= 0 || math.IsNaN(rawScore) || math.IsInf(rawScore, 0) || algorithmVersion == "" || len(algorithmVersion) > 64 {
		return RecallSignal{}, ErrInvalidRecallSignal
	}
	return RecallSignal{documentVersionID: documentVersionID, channel: channel, rank: rank, rawScore: rawScore, algorithmVersion: algorithmVersion}, nil
}

func (signal RecallSignal) DocumentVersionID() int64 { return signal.documentVersionID }
func (signal RecallSignal) Channel() RecallChannel   { return signal.channel }
func (signal RecallSignal) Rank() int                { return signal.rank }
func (signal RecallSignal) RawScore() float64        { return signal.rawScore }
func (signal RecallSignal) AlgorithmVersion() string { return signal.algorithmVersion }

// FusedRecallCandidate is one exact DocumentVersion with every contributing
// channel fact. RRFScore is an ordering score, not a relevance probability.
type FusedRecallCandidate struct {
	documentVersionID int64
	rrfScore          float64
	signals           []RecallSignal
}

func (candidate FusedRecallCandidate) DocumentVersionID() int64 { return candidate.documentVersionID }
func (candidate FusedRecallCandidate) RRFScore() float64        { return candidate.rrfScore }
func (candidate FusedRecallCandidate) Signals() []RecallSignal {
	return append([]RecallSignal(nil), candidate.signals...)
}

// FuseRecallSignals implements Reciprocal Rank Fusion. Only rank contributes
// to the fused score: sum(1/(k+rank)). A document may occur at most once per
// channel, preventing accidental double weighting by an adapter.
func FuseRecallSignals(signals []RecallSignal, rankConstant, limit int) ([]FusedRecallCandidate, error) {
	if rankConstant <= 0 || limit <= 0 || limit > 200 {
		return nil, fmt.Errorf("%w: RRF bounds are invalid", ErrInvalidRecallSignal)
	}
	type accumulator struct {
		score   float64
		signals []RecallSignal
		seen    map[RecallChannel]struct{}
	}
	byDocument := make(map[int64]*accumulator)
	for _, signal := range signals {
		validated, err := NewRecallSignal(signal.documentVersionID, signal.channel, signal.rank, signal.rawScore, signal.algorithmVersion)
		if err != nil {
			return nil, err
		}
		entry := byDocument[validated.documentVersionID]
		if entry == nil {
			entry = &accumulator{seen: make(map[RecallChannel]struct{}, 3)}
			byDocument[validated.documentVersionID] = entry
		}
		if _, duplicate := entry.seen[validated.channel]; duplicate {
			return nil, fmt.Errorf("%w: duplicate document channel", ErrInvalidRecallSignal)
		}
		entry.seen[validated.channel] = struct{}{}
		entry.score += 1 / float64(rankConstant+validated.rank)
		entry.signals = append(entry.signals, validated)
	}
	result := make([]FusedRecallCandidate, 0, len(byDocument))
	for documentVersionID, entry := range byDocument {
		sort.Slice(entry.signals, func(left, right int) bool {
			return entry.signals[left].channel < entry.signals[right].channel
		})
		result = append(result, FusedRecallCandidate{documentVersionID: documentVersionID, rrfScore: entry.score, signals: append([]RecallSignal(nil), entry.signals...)})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].rrfScore != result[right].rrfScore {
			return result[left].rrfScore > result[right].rrfScore
		}
		return result[left].documentVersionID < result[right].documentVersionID
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
