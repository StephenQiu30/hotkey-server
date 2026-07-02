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
	changes, err := ParseWritebackFields(content)
	if err != nil {
		t.Fatalf("parse writeback: %v", err)
	}

	// Should extract both whitelisted fields.
	if len(changes) != 2 {
		t.Fatalf("got %d changes, want 2 (manual_tags + analyst_conclusion)", len(changes))
	}

	// Map by field name for deterministic assertions.
	byField := make(map[string]*WritebackChange)
	for _, ch := range changes {
		byField[ch.FieldName] = ch
	}

	for _, name := range []string{"manual_tags", "analyst_conclusion"} {
		if _, ok := byField[name]; !ok {
			t.Fatalf("missing field %q in parsed changes", name)
		}
	}

	// Verify object identity is consistent across all changes.
	for _, ch := range changes {
		if ch.ObjectType != "theme" {
			t.Fatalf("change %q: got object_type %q, want %q", ch.FieldName, ch.ObjectType, "theme")
		}
		if ch.ObjectID != 201 {
			t.Fatalf("change %q: got object_id %d, want %d", ch.FieldName, ch.ObjectID, 201)
		}
	}

	// Verify manual_tags is parsed as a []string.
	tags := byField["manual_tags"]
	if tagsList, ok := tags.FieldValue.([]string); !ok {
		t.Fatalf("manual_tags value is not []string, got %T", tags.FieldValue)
	} else if len(tagsList) != 1 || tagsList[0] != "ai监管" {
		t.Fatalf("manual_tags: got %v, want [ai监管]", tagsList)
	}
}

func TestParseWritebackFields_ReturnsEmptyWhenNoWhitelistedField(t *testing.T) {
	content := `---
type: hotkey-event
event_id: 301
heat: 88.5
---
`
	changes, err := ParseWritebackFields(content)
	if err != nil {
		t.Fatalf("parse writeback: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected 0 changes for non-whitelisted frontmatter, got %d", len(changes))
	}
}
