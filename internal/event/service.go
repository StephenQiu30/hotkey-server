package event

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

const (
	MatchMethodSeed   = "seed"
	MatchMethodVector = "vector"
	MatchMethodRule   = "rule"

	defaultSimilarityThreshold = 0.82
)

var ErrInvalidCandidate = errors.New("invalid event candidate")

type Options struct {
	VectorEnabled       bool
	SimilarityThreshold float64
}

type CandidateInput struct {
	SourceItemID string    `json:"sourceItemId"`
	Title        string    `json:"title"`
	ContentHash  string    `json:"contentHash"`
	Vector       []float64 `json:"vector"`
}

type ClusterItem struct {
	SourceItemID string    `json:"sourceItemId"`
	Title        string    `json:"title"`
	ContentHash  string    `json:"contentHash"`
	Vector       []float64 `json:"vector,omitempty"`
	MatchMethod  string    `json:"matchMethod"`
	Similarity   float64   `json:"similarity"`
}

type EventCluster struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Items         []ClusterItem `json:"items"`
	MatchMethod   string        `json:"matchMethod"`
	MaxSimilarity float64       `json:"maxSimilarity"`
}

type ClusterMatch struct {
	ClusterID    string  `json:"clusterId"`
	SourceItemID string  `json:"sourceItemId"`
	MatchMethod  string  `json:"matchMethod"`
	Similarity   float64 `json:"similarity"`
}

type Service struct {
	mu                  sync.Mutex
	options             Options
	nextClusterNumber   int
	clusters            map[string]EventCluster
	sourceItemToCluster map[string]string
}

func NewService(options Options) *Service {
	if options.SimilarityThreshold <= 0 {
		options.SimilarityThreshold = defaultSimilarityThreshold
	}
	return &Service{
		options:             options,
		nextClusterNumber:   1,
		clusters:            make(map[string]EventCluster),
		sourceItemToCluster: make(map[string]string),
	}
}

func (s *Service) UpsertCandidate(input CandidateInput) (ClusterMatch, error) {
	item, err := normalizeCandidate(input)
	if err != nil {
		return ClusterMatch{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if clusterID, ok := s.sourceItemToCluster[item.SourceItemID]; ok {
		cluster := s.clusters[clusterID]
		return ClusterMatch{
			ClusterID:    cluster.ID,
			SourceItemID: item.SourceItemID,
			MatchMethod:  item.MatchMethod,
			Similarity:   item.Similarity,
		}, nil
	}

	clusterID, method, similarity, ok := s.findBestCluster(item)
	if !ok {
		clusterID = fmt.Sprintf("cluster_%d", s.nextClusterNumber)
		s.nextClusterNumber++
		item.MatchMethod = MatchMethodSeed
		item.Similarity = 1
		s.clusters[clusterID] = EventCluster{
			ID:            clusterID,
			Title:         item.Title,
			Items:         []ClusterItem{item},
			MatchMethod:   MatchMethodSeed,
			MaxSimilarity: 1,
		}
		s.sourceItemToCluster[item.SourceItemID] = clusterID
		return ClusterMatch{
			ClusterID:    clusterID,
			SourceItemID: item.SourceItemID,
			MatchMethod:  MatchMethodSeed,
			Similarity:   1,
		}, nil
	}

	item.MatchMethod = method
	item.Similarity = similarity
	cluster := s.clusters[clusterID]
	cluster.Items = append(cluster.Items, item)
	cluster.MatchMethod = method
	if similarity > cluster.MaxSimilarity {
		cluster.MaxSimilarity = similarity
	}
	s.clusters[clusterID] = cluster
	s.sourceItemToCluster[item.SourceItemID] = clusterID
	return ClusterMatch{
		ClusterID:    clusterID,
		SourceItemID: item.SourceItemID,
		MatchMethod:  method,
		Similarity:   similarity,
	}, nil
}

func (s *Service) ListClusters() []EventCluster {
	s.mu.Lock()
	defer s.mu.Unlock()

	clusters := make([]EventCluster, 0, len(s.clusters))
	for _, cluster := range s.clusters {
		clusters = append(clusters, cloneCluster(cluster))
	}
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].ID < clusters[j].ID
	})
	return clusters
}

func (s *Service) findBestCluster(item ClusterItem) (string, string, float64, bool) {
	var bestClusterID string
	var bestMethod string
	var bestSimilarity float64

	for _, cluster := range s.clusters {
		for _, existing := range cluster.Items {
			if s.options.VectorEnabled {
				if similarity, ok := cosineSimilarity(item.Vector, existing.Vector); ok && similarity >= s.options.SimilarityThreshold && similarity > bestSimilarity {
					bestClusterID = cluster.ID
					bestMethod = MatchMethodVector
					bestSimilarity = similarity
				}
			}
			if bestClusterID == "" && ruleMatches(item, existing) {
				bestClusterID = cluster.ID
				bestMethod = MatchMethodRule
				bestSimilarity = 1
			}
		}
	}

	if bestClusterID == "" {
		return "", "", 0, false
	}
	return bestClusterID, bestMethod, bestSimilarity, true
}

func normalizeCandidate(input CandidateInput) (ClusterItem, error) {
	sourceItemID := strings.TrimSpace(input.SourceItemID)
	title := strings.Join(strings.Fields(input.Title), " ")
	contentHash := strings.TrimSpace(input.ContentHash)
	if sourceItemID == "" || title == "" || contentHash == "" {
		return ClusterItem{}, ErrInvalidCandidate
	}
	vector := append([]float64(nil), input.Vector...)
	return ClusterItem{
		SourceItemID: sourceItemID,
		Title:        title,
		ContentHash:  contentHash,
		Vector:       vector,
	}, nil
}

func cosineSimilarity(a []float64, b []float64) (float64, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	var dot float64
	var normA float64
	var normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0, false
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), true
}

func ruleMatches(candidate ClusterItem, existing ClusterItem) bool {
	if candidate.ContentHash != "" && candidate.ContentHash == existing.ContentHash {
		return true
	}
	return normalizeTitle(candidate.Title) == normalizeTitle(existing.Title)
}

func normalizeTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

func cloneCluster(cluster EventCluster) EventCluster {
	cluster.Items = append([]ClusterItem(nil), cluster.Items...)
	for i := range cluster.Items {
		cluster.Items[i].Vector = append([]float64(nil), cluster.Items[i].Vector...)
	}
	return cluster
}
