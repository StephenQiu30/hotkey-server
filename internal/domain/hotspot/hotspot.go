package hotspot

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

type EmbeddingStatus string

const (
	EmbeddingStatusSucceeded    EmbeddingStatus = "succeeded"
	EmbeddingStatusFailed       EmbeddingStatus = "failed"
	EmbeddingStatusFailedConfig EmbeddingStatus = "failed_config"
)

var ErrNotFound = errors.New("not found")

type Embedding struct {
	ItemID    string
	Model     string
	Vector    []float64
	TextHash  string
	Status    EmbeddingStatus
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Cluster struct {
	ID          string
	Title       string
	Keywords    []string
	Centroid    []float64
	WindowStart time.Time
	WindowEnd   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ClusterItem struct {
	ClusterID  string
	ItemID     string
	Similarity float64
	CreatedAt  time.Time
}

type Candidate struct {
	Item      content.SourceItem
	Embedding Embedding
}

type MemoryRepository struct {
	mu         sync.RWMutex
	items      map[string]content.SourceItem
	embeddings map[string]Embedding
	clusters   map[string]Cluster
	clusterIDs []string
	links      map[string]map[string]ClusterItem
	nextID     int
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		items:      make(map[string]content.SourceItem),
		embeddings: make(map[string]Embedding),
		clusters:   make(map[string]Cluster),
		links:      make(map[string]map[string]ClusterItem),
	}
}

func (r *MemoryRepository) SaveItem(_ context.Context, item content.SourceItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = cloneItem(item)
	return nil
}

func (r *MemoryRepository) SaveEmbedding(_ context.Context, embedding Embedding) (Embedding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	embedding.Vector = append([]float64(nil), embedding.Vector...)
	if embedding.CreatedAt.IsZero() {
		embedding.CreatedAt = time.Now().UTC()
	}
	if embedding.UpdatedAt.IsZero() {
		embedding.UpdatedAt = embedding.CreatedAt
	}
	r.embeddings[embedding.ItemID] = embedding
	return cloneEmbedding(embedding), nil
}

func (r *MemoryRepository) FindEmbedding(_ context.Context, itemID string) (Embedding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	embedding, ok := r.embeddings[itemID]
	if !ok {
		return Embedding{}, ErrNotFound
	}
	return cloneEmbedding(embedding), nil
}

func (r *MemoryRepository) ListCandidates(_ context.Context, start time.Time, end time.Time) ([]Candidate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	candidates := make([]Candidate, 0, len(r.embeddings))
	for itemID, embedding := range r.embeddings {
		if embedding.Status != EmbeddingStatusSucceeded {
			continue
		}
		item, ok := r.items[itemID]
		if !ok {
			continue
		}
		when := item.CreatedAt
		if item.PublishedAt != nil {
			when = *item.PublishedAt
		}
		if when.Before(start) || !when.Before(end) {
			continue
		}
		candidates = append(candidates, Candidate{Item: cloneItem(item), Embedding: cloneEmbedding(embedding)})
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := effectiveItemTime(candidates[i].Item)
		right := effectiveItemTime(candidates[j].Item)
		if !left.Equal(right) {
			return left.Before(right)
		}
		return candidates[i].Item.ID < candidates[j].Item.ID
	})
	return candidates, nil
}

func (r *MemoryRepository) CreateCluster(_ context.Context, cluster Cluster, items []ClusterItem) (Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cluster.ID == "" {
		r.nextID++
		cluster.ID = "cluster-" + strconvID(r.nextID)
	}
	cluster.Centroid = append([]float64(nil), cluster.Centroid...)
	cluster.Keywords = append([]string(nil), cluster.Keywords...)
	r.clusters[cluster.ID] = cluster
	if !containsString(r.clusterIDs, cluster.ID) {
		r.clusterIDs = append(r.clusterIDs, cluster.ID)
	}
	if r.links[cluster.ID] == nil {
		r.links[cluster.ID] = make(map[string]ClusterItem)
	}
	for _, item := range items {
		item.ClusterID = cluster.ID
		r.links[cluster.ID][item.ItemID] = item
	}
	return cloneCluster(cluster), nil
}

func (r *MemoryRepository) ReplaceClusters(_ context.Context, clusters []Cluster, itemsByCluster map[string][]ClusterItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters = make(map[string]Cluster)
	r.clusterIDs = r.clusterIDs[:0]
	r.links = make(map[string]map[string]ClusterItem)
	for _, cluster := range clusters {
		r.clusters[cluster.ID] = cloneCluster(cluster)
		r.clusterIDs = append(r.clusterIDs, cluster.ID)
		r.links[cluster.ID] = make(map[string]ClusterItem)
		for _, item := range itemsByCluster[cluster.ID] {
			r.links[cluster.ID][item.ItemID] = item
		}
	}
	return nil
}

func CosineSimilarity(a []float64, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func cloneEmbedding(embedding Embedding) Embedding {
	embedding.Vector = append([]float64(nil), embedding.Vector...)
	return embedding
}

func cloneCluster(cluster Cluster) Cluster {
	cluster.Keywords = append([]string(nil), cluster.Keywords...)
	cluster.Centroid = append([]float64(nil), cluster.Centroid...)
	return cluster
}

func cloneItem(item content.SourceItem) content.SourceItem {
	if item.PublishedAt != nil {
		publishedAt := *item.PublishedAt
		item.PublishedAt = &publishedAt
	}
	return item
}

func effectiveItemTime(item content.SourceItem) time.Time {
	if item.PublishedAt != nil {
		return *item.PublishedAt
	}
	return item.CreatedAt
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func strconvID(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
