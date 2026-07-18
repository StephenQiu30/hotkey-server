package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestCapturePolicyStoresFeedBodyByDefaultForRSSAndHackerNews(t *testing.T) {
	observedAt := time.Date(2026, time.July, 16, 11, 0, 0, 0, time.UTC)
	connections := []domain.SourceConnection{
		{SourceType: domain.SourceTypeRSS, Config: domain.DefaultSourceConfig()},
		{SourceType: domain.SourceTypeHackerNews, Config: domain.DefaultSourceConfig()},
	}
	items := []domain.SourceItem{
		{SourceCode: "rss", ExternalID: "rss-1", ContentType: "article", Body: "must not persist", ObservedAt: observedAt, Metrics: domain.SourceMetrics{ViewCount: domain.KnownMetric(7), CommentCount: domain.KnownMetric(2)}, RawPayload: []byte(`{"secret":"rss"}`)},
		{SourceCode: "hacker_news", ExternalID: "hn-1", ContentType: "article", Body: "must not persist", ObservedAt: observedAt, Metrics: domain.SourceMetrics{ViewCount: domain.KnownMetric(7), CommentCount: domain.KnownMetric(2)}, RawPayload: []byte(`{"secret":"hn"}`)},
	}
	var captures []domain.CapturedItem
	for index, connection := range connections {
		captured, err := capturePolicy(connection).Capture(items[index])
		if err != nil {
			t.Fatalf("Capture(%q): %v", connection.SourceType, err)
		}
		captures = append(captures, captured)
	}
	for _, captured := range captures {
		if captured.Body != "must not persist" || len(captured.RawPayload) != 0 || captured.RawPayloadDisposition != domain.RawPayloadCapturedItemOnly {
			t.Fatalf("captured projection = %#v, want Feed body and no raw payload", captured)
		}
	}
	if !reflect.DeepEqual(captures[0].Metrics, captures[1].Metrics) || captures[0].RawPayloadDisposition != captures[1].RawPayloadDisposition {
		t.Fatalf("source capture policy diverged: %#v / %#v", captures[0], captures[1])
	}
}
