package obsidian

import (
	"testing"
)

func TestParseWritebackFields(t *testing.T) {
	content := `---
type: hotkey-theme
theme_id: 201
manual_tags:
  - ai监管
analyst_conclusion: 持续关注监管升级
---
`
	parsed, err := ParseWritebackFields(content)
	if err != nil {
		t.Fatalf("parse writeback: %v", err)
	}
	if parsed.ObjectType != "theme" {
		t.Fatalf("got object_type %q, want %q", parsed.ObjectType, "theme")
	}
	if parsed.ObjectID != 201 {
		t.Fatalf("got object_id %d, want %d", parsed.ObjectID, 201)
	}
}
