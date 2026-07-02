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

// ParseWritebackFields extracts whitelisted writeback fields from Obsidian
// Markdown content with YAML frontmatter.
func ParseWritebackFields(content string) (*WritebackChange, error) {
	frontmatter, err := extractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	meta := parseKeyValues(frontmatter)

	change := &WritebackChange{}

	if v, ok := meta["type"]; ok {
		parts := strings.SplitN(v, "-", 2)
		if len(parts) == 2 && parts[0] == "hotkey" {
			change.ObjectType = parts[1]
		} else {
			change.ObjectType = v
		}
	}

	if v, ok := meta["topic_id"]; ok {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			change.ObjectID = id
		}
	}

	if change.ObjectID == 0 {
		if v, ok := meta["theme_id"]; ok {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				change.ObjectID = id
			}
		}
	}

	if change.ObjectID == 0 {
		if v, ok := meta["monitor_id"]; ok {
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				change.ObjectID = id
			}
		}
	}

	// Scan for whitelisted fields
	allowedFields := map[string]bool{
		"manual_tags":        true,
		"analyst_conclusion": true,
		"theme_ref":          true,
		"material_status":    true,
	}

	for field, value := range meta {
		if allowedFields[field] {
			// Try to parse YAML list values
			if strings.HasPrefix(value, "\n") || strings.Contains(value, "\n  - ") {
				items := parseYAMLList(value)
				change.FieldName = field
				change.FieldValue = items
				return change, nil
			}
			change.FieldName = field
			change.FieldValue = value
			return change, nil
		}
	}

	return nil, fmt.Errorf("no whitelisted writeback fields found")
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
			result[currentKey] = "\n" + strings.Join(listValues, "\n  - ")
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
		result[currentKey] = "\n" + strings.Join(listValues, "\n  - ")
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
