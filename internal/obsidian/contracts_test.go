package obsidian

import (
	"testing"
)

func TestKnowledgeContract_MinimumFields(t *testing.T) {
	contract := BuildEventContract(EventContractInput{
		EventID:  101,
		EventKey: "evt:ai-regulation:2026-07-01",
		Title:    "AI 监管规则发布",
		TopicIDs: []int64{42},
		Date:     "2026-07-01",
	})
	if contract.Frontmatter["event_id"] != int64(101) {
		t.Fatal("expected event_id frontmatter")
	}
	if contract.Frontmatter["type"] != "hotkey-event" {
		t.Fatalf("expected type hotkey-event, got %v", contract.Frontmatter["type"])
	}
}

func TestBuildRevision(t *testing.T) {
	rev := BuildRevision("event", 101, "some content")
	if rev == "" {
		t.Fatal("expected non-empty revision")
	}
	rev2 := BuildRevision("event", 101, "some content")
	if rev != rev2 {
		t.Fatal("same content should produce same revision")
	}
}

func TestBuildRevisionDiffersForDifferentContent(t *testing.T) {
	rev1 := BuildRevision("event", 101, "content A")
	rev2 := BuildRevision("event", 101, "content B")
	if rev1 == rev2 {
		t.Fatal("different content should produce different revisions")
	}
}
