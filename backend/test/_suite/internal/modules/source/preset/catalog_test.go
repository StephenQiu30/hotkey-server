package preset

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestCatalogKeepsFreePresetsFirstAndConnectionDetailsServerSide(t *testing.T) {
	items := Catalog()
	wantIDs := []string{
		"rss_custom", "youtube_channel", "github_releases", "arxiv_search",
		"mastodon_account", "mastodon_hashtag", "hacker_news", "x",
		"bing_grounding", "bilibili", "weibo", "google_agent_search",
	}
	if len(items) != len(wantIDs) {
		t.Fatalf("len(Catalog()) = %d, want %d", len(items), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		if items[index].ID != wantID {
			t.Fatalf("Catalog()[%d].ID = %q, want %q", index, items[index].ID, wantID)
		}
		if items[index].Label == "" || items[index].SourceType == "" || items[index].AuthLabel == "" {
			t.Fatalf("Catalog()[%d] has incomplete public metadata: %#v", index, items[index])
		}
	}
	if items[0].Cost != CostFree || items[5].Cost != CostFree || items[7].Cost != CostPaid {
		t.Fatalf("catalog cost ordering is not free-first: %#v", items)
	}
}

func TestResolveBuildsFreeFeedConnectionsOnTheServer(t *testing.T) {
	tests := []struct {
		id       string
		values   map[string]string
		endpoint string
	}{
		{id: "youtube_channel", values: map[string]string{"youtube_channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw"}, endpoint: "https://www.youtube.com/feeds/videos.xml?channel_id=UC_x5XG1OV2P6uZZ5FSM9Ttw"},
		{id: "github_releases", values: map[string]string{"github_repository": "openai/openai-node"}, endpoint: "https://github.com/openai/openai-node/releases.atom"},
		{id: "arxiv_search", values: map[string]string{"arxiv_query": "graph neural networks"}, endpoint: "https://export.arxiv.org/api/query?search_query=all%3Agraph%20neural%20networks&start=0&max_results=100"},
		{id: "mastodon_account", values: map[string]string{"mastodon_instance": "mastodon.social", "mastodon_value": "@Gargron"}, endpoint: "https://mastodon.social/@Gargron.rss"},
		{id: "mastodon_hashtag", values: map[string]string{"mastodon_instance": "mastodon.social", "mastodon_value": "#opensource"}, endpoint: "https://mastodon.social/tags/opensource.rss"},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			connection, err := Resolve(test.id, "Personal feed", test.values, domain.DefaultSourceConfig())
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if connection.SourceType != domain.SourceTypeRSS || connection.AuthType != domain.AuthTypeNone || !connection.Enabled || connection.Endpoint != test.endpoint {
				t.Fatalf("Resolve() = %#v, want enabled RSS %q", connection, test.endpoint)
			}
		})
	}
}

func TestResolveRejectsUnknownOrMalformedPresetValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		id     string
		values map[string]string
	}{
		{name: "unknown preset", id: "free_x_scraper"},
		{name: "unknown value", id: "youtube_channel", values: map[string]string{"cookie": "secret"}},
		{name: "invalid channel", id: "youtube_channel", values: map[string]string{"youtube_channel_id": "not-a-channel"}},
		{name: "invalid mastodon instance", id: "mastodon_account", values: map[string]string{"mastodon_instance": "https://mastodon.social/path", "mastodon_value": "user"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Resolve(test.id, "Rejected", test.values, domain.DefaultSourceConfig()); err == nil {
				t.Fatal("Resolve() error = nil, want invalid preset error")
			}
		})
	}
}
