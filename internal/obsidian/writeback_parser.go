package obsidian

import (
	"fmt"
	"strconv"
	"strings"
)

// WritebackChange represents a parsed writeback change from an Obsidian note.
type WritebackChange struct {
	ObjectType string // "theme", "event", "topic"
	ObjectID   int64
	FieldName  string
	FieldValue interface{}
}

// ParseWritebackFields extracts all whitelisted writeback fields from Obsidian
// Markdown content with YAML frontmatter. Returns all fields found, or nil slice
// if no whitelisted field is present (caller should check length).
func ParseWritebackFields(content string) ([]*WritebackChange, error) {
	frontmatter, err := extractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	meta := parseKeyValues(frontmatter)

	objectType := resolveObjectType(meta)
	objectID := resolveObjectID(meta)

	// Scan for whitelisted fields — collect ALL matches, not just the first.
	allowedFields := map[string]bool{
		"manual_tags":        true,
		"analyst_conclusion": true,
		"theme_ref":          true,
		"material_status":    true,
	}

	var changes []*WritebackChange
	for field, value := range meta {
		if !allowedFields[field] {
			continue
		}
		change := &WritebackChange{
			ObjectType: objectType,
			ObjectID:   objectID,
			FieldName:  field,
		}
		if strings.HasPrefix(value, "\n") || strings.Contains(value, "\n  - ") {
			change.FieldValue = parseYAMLList(value)
		} else {
			change.FieldValue = value
		}
		changes = append(changes, change)
	}

	return changes, nil
}

// resolveObjectType extracts the object type from frontmatter meta.
func resolveObjectType(meta map[string]string) string {
	v, ok := meta["type"]
	if !ok {
		return ""
	}
	parts := strings.SplitN(v, "-", 2)
	if len(parts) == 2 && parts[0] == "hotkey" {
		return parts[1]
	}
	return v
}

// resolveObjectID extracts the numeric object ID from frontmatter meta.
// Priority: topic_id > theme_id > monitor_id.
func resolveObjectID(meta map[string]string) int64 {
	for _, key := range []string{"topic_id", "theme_id", "monitor_id"} {
		if v, ok := meta[key]; ok {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				return id
			}
		}
	}
	return 0
}

// extractFrontmatter extracts the YAML frontmatter block from Markdown content.
func extractFrontmatter(content string) (string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", fmt.Errorf("content does not start with frontmatter")
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		endIdx = strings.Index(rest, "\n---\n")
	}
	if endIdx < 0 {
		return "", fmt.Errorf("unclosed frontmatter")
	}

	return rest[:endIdx], nil
}

// parseKeyValues parses simple YAML key-value pairs.
// Supports:
//   - key: value
//   - key: "quoted value"
//   - key:
//       - list_item
func parseKeyValues(frontmatter string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(frontmatter, "\n")

	var currentKey string
	var listValues []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			listValues = append(listValues, strings.TrimPrefix(trimmed, "- "))
			continue
		}

		// Flush any pending list
		if len(listValues) > 0 {
			result[currentKey] = "\n  - " + strings.Join(listValues, "\n  - ")
			listValues = nil
			currentKey = ""
		}

		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])
		val = strings.Trim(val, "\"")
		currentKey = key

		if val == "" {
			// Start collecting list items
			listValues = nil
		} else {
			result[key] = val
		}
	}

	// Flush trailing list
	if len(listValues) > 0 {
		result[currentKey] = "\n  - " + strings.Join(listValues, "\n  - ")
	}

	return result
}

// parseYAMLList parses a YAML list value into a []string.
func parseYAMLList(value string) []string {
	value = strings.TrimPrefix(value, "\n")
	var items []string
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			items = append(items, strings.TrimPrefix(trimmed, "- "))
		}
	}
	return items
}
