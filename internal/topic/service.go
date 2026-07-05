// Package topic implements token-based Jaccard similarity clustering.
package topic

import (
	"sort"
	"strings"
)

type CandidatePost struct {
	PostID int64
	Tokens []string
}

type Topic struct {
	TopicKey  string
	Title     string
	PostIDs   []int64
	Tokens    []string // merged token set for the cluster
}

type Repository interface {
	UpsertTopic(monitorID int64, t Topic) (topicID int64, err error)
	AddPostToTopic(topicID, postID int64, membershipScore float64) error
	ListByMonitor(monitorID int64) ([]TopicSummary, error)
}

type TopicSummary struct {
	ID             int64   `json:"id"`
	Title          string  `json:"title"`
	Summary        string  `json:"summary"`
	CurrentHeat    float64 `json:"current_heat"`
	TrendDirection string  `json:"trend_direction"`
	PostCount      int     `json:"post_count"`
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

const similarityThreshold = 0.3

// Cluster groups posts into topics using Jaccard similarity >= threshold.
func (s *Service) Cluster(posts []CandidatePost) []Topic {
	if len(posts) == 0 {
		return nil
	}

	// Union-Find for clustering
	parent := make([]int, len(posts))
	for i := range parent {
		parent[i] = i
	}

	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for i := 0; i < len(posts); i++ {
		for j := i + 1; j < len(posts); j++ {
			sim := JaccardSimilarity(posts[i].Tokens, posts[j].Tokens)
			if sim >= similarityThreshold {
				union(i, j)
			}
		}
	}

	groups := make(map[int][]int)
	for i := range posts {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	topics := make([]Topic, 0, len(groups))
	for _, indices := range groups {
		postIDs := make([]int64, 0, len(indices))
		tokenSet := make(map[string]struct{})
		for _, idx := range indices {
			postIDs = append(postIDs, posts[idx].PostID)
			for _, tok := range posts[idx].Tokens {
				tokenSet[tok] = struct{}{}
			}
		}
		merged := make([]string, 0, len(tokenSet))
		for tok := range tokenSet {
			merged = append(merged, tok)
		}
		sort.Strings(merged)
		sort.Slice(postIDs, func(i, j int) bool { return postIDs[i] < postIDs[j] })

		title := generateTitle(merged)
		topics = append(topics, Topic{
			TopicKey: generateTopicKey(merged),
			Title:    title,
			PostIDs:  postIDs,
			Tokens:   merged,
		})
	}

	return topics
}

// JaccardSimilarity returns |intersection| / |union| of two token sets.
func JaccardSimilarity(a, b []string) float64 {
	setA := make(map[string]struct{}, len(a))
	for _, t := range a {
		setA[t] = struct{}{}
	}
	setB := make(map[string]struct{}, len(b))
	for _, t := range b {
		setB[t] = struct{}{}
	}

	intersection := 0
	for t := range setA {
		if _, ok := setB[t]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// ExtractTokens splits text into lowercase word tokens.
func ExtractTokens(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	tokens := make([]string, 0, len(words))
	for _, w := range words {
		cleaned := strings.Trim(w, ".,!?;:()[]{}\"'")
		if cleaned != "" && len(cleaned) > 1 {
			tokens = append(tokens, cleaned)
		}
	}
	return tokens
}

func generateTopicKey(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	limit := 3
	if len(tokens) < limit {
		limit = len(tokens)
	}
	return strings.Join(tokens[:limit], ":")
}

func generateTitle(tokens []string) string {
	if len(tokens) == 0 {
		return "Untitled"
	}
	limit := 5
	if len(tokens) < limit {
		limit = len(tokens)
	}
	return strings.Join(tokens[:limit], " / ")
}
