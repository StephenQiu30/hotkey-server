package application

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestFilterCollectionItemsAppliesMonitorTermsAndHardExcludesBeforeCapture(t *testing.T) {
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
	if len(filtered) != 1 || filtered[0].ExternalID != "accepted" {
		t.Fatalf("filtered items = %#v, want only relevant non-excluded content", filtered)
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

func TestFilterCollectionItemsAcceptsStrongTokenCoverage(t *testing.T) {
	t.Parallel()

	items := []domain.SourceItem{
		{ExternalID: "covered", Title: "OpenAI ships a new reasoning model"},
		{ExternalID: "weak", Title: "OpenAI updates its status page"},
	}
	filtered := filterCollectionItems(items, []domain.CollectionTerm{{Value: "OpenAI reasoning model"}})
	if len(filtered) != 1 || filtered[0].ExternalID != "covered" {
		t.Fatalf("filtered items = %#v, want the strong token match only", filtered)
	}
}
