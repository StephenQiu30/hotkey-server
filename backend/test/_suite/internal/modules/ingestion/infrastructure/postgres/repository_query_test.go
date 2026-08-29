package postgres

import (
	"strings"
	"testing"
	"time"

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
		{sort: ingestiondomain.ContentSortDiscovered, want: "ORDER BY CASE WHEN snapshot_metric.id IS NULL THEN c.fetched_at ELSE snapshot_metric.captured_at END DESC, c.id DESC"},
		{sort: ingestiondomain.ContentSortPublished, want: "ORDER BY COALESCE(c.published_at, '-infinity'::timestamptz) DESC, c.id DESC"},
		{sort: ingestiondomain.ContentSortImportance, want: "hotspot_heat_score(CASE WHEN snapshot_metric.id IS NULL THEN c.view_count"},
		{sort: ingestiondomain.ContentSortHeat, want: "hotspot_heat_score(CASE WHEN snapshot_metric.id IS NULL THEN c.view_count"},
		{sort: ingestiondomain.ContentSortRelevance, monitorID: &monitorID, want: "ORDER BY latest_match.final_score DESC, c.id DESC"},
	}
	for _, test := range tests {
		query := ingestiondomain.ContentListQuery{Limit: 20, Sort: test.sort, MonitorID: test.monitorID}
		cursor := contentListCursor{AsOf: time.Now(), ID: 42, Timestamp: time.Now(), Score: 50}
		statement, _ := contentListStatement(query, cursor)
		if !strings.Contains(statement, test.want) {
			t.Fatalf("sort %q statement does not contain %q:\n%s", query.Sort, test.want, statement)
		}
		if (test.sort == ingestiondomain.ContentSortLatest || test.sort == ingestiondomain.ContentSortPublished) && (!strings.Contains(statement, "COALESCE(c.published_at, '-infinity'::timestamptz)") || strings.Contains(statement, "FROM contents AS previous")) {
			t.Fatalf("published cursor does not carry its immutable boundary:\n%s", statement)
		}
		if !strings.Contains(statement, "c.created_at <=") || !strings.Contains(statement, "metric.created_at <=") {
			t.Fatalf("sort %q statement does not pin the traversal snapshot:\n%s", query.Sort, statement)
		}
		if !strings.Contains(statement, activeContentRightsVisibilityCondition) {
			t.Fatalf("sort %q statement does not fail closed on current Content rights:\n%s", query.Sort, statement)
		}
	}
}
