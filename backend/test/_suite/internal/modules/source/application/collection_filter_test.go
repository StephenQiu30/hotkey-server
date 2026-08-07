package application

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestFilterCollectionItemsAppliesHardExcludesBeforeCaptureWithoutReducingRecall(t *testing.T) {
	t.Parallel()

	items := []domain.SourceItem{
		{ExternalID: "accepted", Title: "OpenAI releases a new model", Body: "technical details"},
		{ExternalID: "excluded", Title: "OpenAI job listing", Body: "careers"},
		{ExternalID: "unrelated", Title: "Weather report", Body: "sunny"},
	}
	filtered := filterCollectionItems(items, []domain.CollectionTerm{
		{Value: "OpenAI"},
		{Value: "job listing", Excluded: true},
	})
	if len(filtered) != 2 || filtered[0].ExternalID != "accepted" || filtered[1].ExternalID != "unrelated" {
		t.Fatalf("filtered items = %#v, want the hard exclude removed without dropping source-scope recall", filtered)
	}
}

func TestFilterCollectionItemsKeepsManualRulesIndependentOfAI(t *testing.T) {
	t.Parallel()

	items := []domain.SourceItem{{ExternalID: "manual", Title: "人工智能产品发布"}}
	filtered := filterCollectionItems(items, []domain.CollectionTerm{{Value: "人工智能"}})
	if len(filtered) != 1 || filtered[0].ExternalID != "manual" {
		t.Fatalf("manual-only filtered items = %#v", filtered)
	}
}
