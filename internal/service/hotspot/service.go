package hotspot

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"

	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

type Config struct {
	SimilarityThreshold     float64
	KeywordOverlapThreshold int
	Window                  time.Duration
}

type Window struct {
	Start time.Time
	End   time.Time
}

type Repository interface {
	ListCandidates(context.Context, time.Time, time.Time) ([]domainhotspot.Candidate, error)
	ReplaceClusters(context.Context, []domainhotspot.Cluster, map[string][]domainhotspot.ClusterItem) error
}

type Service struct {
	cfg  Config
	repo Repository
	now  func() time.Time
}

type Result struct {
	Clusters       []domainhotspot.Cluster
	ItemsByCluster map[string][]domainhotspot.ClusterItem
}

type clusterDraft struct {
	cluster domainhotspot.Cluster
	items   []domainhotspot.ClusterItem
	vectors [][]float64
	words   map[string]struct{}
}

func NewService(cfg Config, repo Repository) *Service {
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = 0.82
	}
	if cfg.KeywordOverlapThreshold <= 0 {
		cfg.KeywordOverlapThreshold = 1
	}
	if cfg.Window <= 0 {
		cfg.Window = 24 * time.Hour
	}
	return &Service{cfg: cfg, repo: repo, now: time.Now}
}

func (s *Service) Cluster(ctx context.Context, window Window) (Result, error) {
	if window.End.IsZero() {
		window.End = s.now().UTC()
	}
	if window.Start.IsZero() {
		window.Start = window.End.Add(-s.cfg.Window)
	}
	candidates, err := s.repo.ListCandidates(ctx, window.Start, window.End)
	if err != nil {
		return Result{}, err
	}
	drafts := make([]clusterDraft, 0, len(candidates))
	for _, candidate := range candidates {
		words := keywordSet(candidate.Item.Title + " " + candidate.Item.Snippet)
		bestIndex := -1
		bestSimilarity := 0.0
		for i := range drafts {
			if overlapCount(words, drafts[i].words) < s.cfg.KeywordOverlapThreshold {
				continue
			}
			similarity := domainhotspot.CosineSimilarity(candidate.Embedding.Vector, drafts[i].cluster.Centroid)
			if similarity >= s.cfg.SimilarityThreshold && similarity > bestSimilarity {
				bestIndex = i
				bestSimilarity = similarity
			}
		}
		if bestIndex == -1 {
			clusterID := "cluster-" + candidate.Item.ID
			drafts = append(drafts, clusterDraft{
				cluster: domainhotspot.Cluster{
					ID:          clusterID,
					Title:       candidate.Item.Title,
					Keywords:    sortedKeywords(words),
					Centroid:    append([]float64(nil), candidate.Embedding.Vector...),
					WindowStart: window.Start,
					WindowEnd:   window.End,
					CreatedAt:   s.now().UTC(),
					UpdatedAt:   s.now().UTC(),
				},
				items: []domainhotspot.ClusterItem{{
					ClusterID:  clusterID,
					ItemID:     candidate.Item.ID,
					Similarity: 1,
					CreatedAt:  s.now().UTC(),
				}},
				vectors: [][]float64{append([]float64(nil), candidate.Embedding.Vector...)},
				words:   words,
			})
			continue
		}
		draft := &drafts[bestIndex]
		draft.items = append(draft.items, domainhotspot.ClusterItem{
			ClusterID:  draft.cluster.ID,
			ItemID:     candidate.Item.ID,
			Similarity: bestSimilarity,
			CreatedAt:  s.now().UTC(),
		})
		draft.vectors = append(draft.vectors, append([]float64(nil), candidate.Embedding.Vector...))
		for word := range words {
			draft.words[word] = struct{}{}
		}
		draft.cluster.Keywords = sortedKeywords(draft.words)
		draft.cluster.Centroid = centroid(draft.vectors)
		draft.cluster.UpdatedAt = s.now().UTC()
	}

	result := Result{ItemsByCluster: make(map[string][]domainhotspot.ClusterItem)}
	for _, draft := range drafts {
		result.Clusters = append(result.Clusters, draft.cluster)
		result.ItemsByCluster[draft.cluster.ID] = append([]domainhotspot.ClusterItem(nil), draft.items...)
	}
	if err := s.repo.ReplaceClusters(ctx, result.Clusters, result.ItemsByCluster); err != nil {
		return Result{}, err
	}
	return result, nil
}

func keywordSet(value string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	words := make(map[string]struct{})
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		words[field] = struct{}{}
	}
	return words
}

func overlapCount(a map[string]struct{}, b map[string]struct{}) int {
	count := 0
	for word := range a {
		if _, ok := b[word]; ok {
			count++
		}
	}
	return count
}

func sortedKeywords(words map[string]struct{}) []string {
	keywords := make([]string, 0, len(words))
	for word := range words {
		keywords = append(keywords, word)
	}
	sort.Strings(keywords)
	return keywords
}

func centroid(vectors [][]float64) []float64 {
	if len(vectors) == 0 {
		return nil
	}
	sum := make([]float64, len(vectors[0]))
	for _, vector := range vectors {
		for i := range sum {
			if i < len(vector) {
				sum[i] += vector[i]
			}
		}
	}
	for i := range sum {
		sum[i] /= float64(len(vectors))
	}
	return sum
}
