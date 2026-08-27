package postgres

import (
	"strings"
	"testing"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

func TestContentListStatementUsesDatabaseOrderingForEveryHotspotSort(t *testing.T) {
	monitorID := int64(7)
	tests := []struct {
		sort      ingestiondomain.ContentSort
		monitorID *int64
		want      string
	}{
		{sort: ingestiondomain.ContentSortLatest, want: "ORDER BY COALESCE(c.published_at, '-infinity'::timestamptz) DESC, c.id DESC"},
		{sort: ingestiondomain.ContentSortDiscovered, want: "ORDER BY c.fetched_at DESC, c.id DESC"},
		{sort: ingestiondomain.ContentSortPublished, want: "ORDER BY COALESCE(c.published_at, '-infinity'::timestamptz) DESC, c.id DESC"},
		{sort: ingestiondomain.ContentSortImportance, want: "hotspot_heat_score(c.view_count,c.like_count,c.comment_count,c.share_count)"},
		{sort: ingestiondomain.ContentSortHeat, want: "hotspot_heat_score(c.view_count,c.like_count,c.comment_count,c.share_count)"},
		{sort: ingestiondomain.ContentSortRelevance, monitorID: &monitorID, want: "ORDER BY latest_match.final_score DESC, c.id DESC"},
	}
	for _, test := range tests {
		query := ingestiondomain.ContentListQuery{Limit: 20, Sort: test.sort, MonitorID: test.monitorID}
		statement, _ := contentListStatement(query, 42)
		if !strings.Contains(statement, test.want) {
			t.Fatalf("sort %q statement does not contain %q:\n%s", query.Sort, test.want, statement)
		}
		if (test.sort == ingestiondomain.ContentSortLatest || test.sort == ingestiondomain.ContentSortPublished) && !strings.Contains(statement, "COALESCE(previous.published_at, '-infinity'::timestamptz)") {
			t.Fatalf("published cursor does not preserve unknown publication times:\n%s", statement)
		}
	}
}
