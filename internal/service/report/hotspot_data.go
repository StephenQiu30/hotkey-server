package report

import (
	"context"
	"sort"
	"strings"
)

type filter struct {
	channelIDs []string
	keywords   []string
}

func (s *Service) gatherHotspotData(ctx context.Context, channelID string, userID string, f filter) ([]HotspotData, error) {
	if s.clusters == nil {
		return nil, nil
	}
	clusters, err := s.clusters.ListClusters(ctx)
	if err != nil {
		return nil, err
	}
	scoreByCluster := map[string]ScoreInfo{}
	if s.scores != nil {
		scores, err := s.scores.ListScores(ctx)
		if err != nil {
			return nil, err
		}
		for _, score := range scores {
			scoreByCluster[score.ClusterID] = score
		}
	}
	sourceByID := map[string]SourceInfo{}
	if s.sources != nil {
		sources, err := s.sources.ListSources(ctx)
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			sourceByID[source.ID] = source
		}
	}
	result := make([]HotspotData, 0, len(clusters))
	for _, cluster := range clusters {
		items, err := s.clusters.ListClusterItems(ctx, cluster.ID)
		if err != nil {
			return nil, err
		}
		data := HotspotData{Cluster: cluster, Score: scoreByCluster[cluster.ID]}
		seenSource := map[string]struct{}{}
		for _, item := range items {
			source := sourceByID[item.SourceID]
			if channelID != "" && !contains(source.ChannelIDs, channelID) {
				continue
			}
			if len(f.channelIDs) > 0 && !intersects(source.ChannelIDs, f.channelIDs) {
				continue
			}
			if len(f.keywords) > 0 && !matchesKeywords(cluster, item, f.keywords) {
				continue
			}
			data.Items = append(data.Items, item)
			if _, ok := seenSource[item.SourceID]; !ok && item.SourceID != "" {
				data.Sources = append(data.Sources, source)
				seenSource[item.SourceID] = struct{}{}
			}
		}
		if len(data.Items) > 0 {
			result = append(result, data)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score.TotalScore != result[j].Score.TotalScore {
			return result[i].Score.TotalScore > result[j].Score.TotalScore
		}
		return result[i].Cluster.UpdatedAt.After(result[j].Cluster.UpdatedAt)
	})
	return result, nil
}

func (s *Service) userFilters(ctx context.Context, userID string) ([]string, []string, error) {
	if s.prefs == nil {
		return nil, nil, nil
	}
	channelIDs, err := s.prefs.ListUserChannelIDs(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	keywords, err := s.prefs.ListUserKeywords(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return channelIDs, keywords, nil
}

func sourceRefs(hotspots []HotspotData) []SourceRef {
	refs := []SourceRef{}
	seen := map[string]struct{}{}
	for _, hotspot := range hotspots {
		for _, item := range hotspot.Items {
			key := item.SourceID + ":" + item.ID
			if _, ok := seen[key]; ok {
				continue
			}
			refs = append(refs, SourceRef{
				SourceID: item.SourceID,
				ItemID:   item.ID,
				Title:    item.Title,
				URL:      item.URL,
			})
			seen[key] = struct{}{}
		}
	}
	return refs
}

func hotspotIDs(hotspots []HotspotData) []string {
	ids := make([]string, 0, len(hotspots))
	for _, hotspot := range hotspots {
		ids = append(ids, hotspot.Cluster.ID)
	}
	return ids
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func intersects(left []string, right []string) bool {
	for _, value := range left {
		if contains(right, value) {
			return true
		}
	}
	return false
}

func matchesKeywords(cluster ClusterInfo, item ContentItemInfo, keywords []string) bool {
	text := strings.ToLower(cluster.Title + " " + strings.Join(cluster.Keywords, " ") + " " + item.Title + " " + item.Snippet)
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return len(keywords) == 0
}
