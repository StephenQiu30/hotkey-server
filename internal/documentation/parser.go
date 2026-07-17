package docgovernance

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

type documentHeader struct {
	Layer           string   `yaml:"layer"`
	DocNo           string   `yaml:"doc_no"`
	Audience        any      `yaml:"audience"`
	FeatureArea     string   `yaml:"feature_area"`
	Purpose         string   `yaml:"purpose"`
	CanonicalPath   string   `yaml:"canonical_path"`
	Status          string   `yaml:"status"`
	Version         string   `yaml:"version"`
	Owner           string   `yaml:"owner"`
	Inputs          []string `yaml:"inputs"`
	Outputs         []string `yaml:"outputs"`
	Triggers        []string `yaml:"triggers"`
	Downstream      []string `yaml:"downstream"`
	ExecutionStatus string   `yaml:"execution_status"`
	ReviewStatus    string   `yaml:"review_status"`
	Result          string   `yaml:"result"`
}

func parseDocument(path string, contents []byte) (Document, map[string]any, string, error) {
	frontmatter, body, err := splitFrontmatter(contents)
	if err != nil {
		return Document{}, nil, "", err
	}
	header := documentHeader{}
	if err := yaml.Unmarshal(frontmatter, &header); err != nil {
		return Document{}, nil, "", fmt.Errorf("decode frontmatter: %w", err)
	}
	fields := map[string]any{}
	if err := yaml.Unmarshal(frontmatter, &fields); err != nil {
		return Document{}, nil, "", fmt.Errorf("inspect frontmatter: %w", err)
	}
	audience, err := normalizeStringList(header.Audience)
	if err != nil {
		return Document{}, nil, "", fmt.Errorf("decode audience: %w", err)
	}
	return Document{
		Path: filepath.ToSlash(path), Layer: header.Layer, DocNo: header.DocNo,
		Audience: audience, FeatureArea: header.FeatureArea, Purpose: header.Purpose,
		CanonicalPath: header.CanonicalPath, Status: header.Status, Version: header.Version,
		Owner: header.Owner, Inputs: header.Inputs, Outputs: header.Outputs,
		Triggers: header.Triggers, Downstream: header.Downstream,
		ExecutionStatus: header.ExecutionStatus, ReviewStatus: header.ReviewStatus, Result: header.Result,
	}, fields, body, nil
}

func normalizeStringList(value any) ([]string, error) {
	switch values := value.(type) {
	case nil:
		return nil, nil
	case string:
		parts := strings.Split(values, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if normalized := strings.TrimSpace(part); normalized != "" {
				result = append(result, normalized)
			}
		}
		return result, nil
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("must contain only strings")
			}
			if normalized := strings.TrimSpace(text); normalized != "" {
				result = append(result, normalized)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("must be a string or string list")
	}
}

func splitFrontmatter(contents []byte) ([]byte, string, error) {
	contents = bytes.TrimPrefix(contents, []byte("\xef\xbb\xbf"))
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, "", fmt.Errorf("frontmatter must begin at the first line")
	}
	rest := contents[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return nil, "", fmt.Errorf("frontmatter closing delimiter is required")
	}
	return rest[:end], string(rest[end+len("\n---\n"):]), nil
}

func parseMarkdownLinkTargets(body string) []string {
	matches := markdownLinkPattern.FindAllStringSubmatch(body, -1)
	targets := make([]string, 0, len(matches))
	for _, match := range matches {
		target := strings.TrimSpace(match[1])
		if strings.HasPrefix(target, "<") {
			if end := strings.Index(target, ">"); end > 1 {
				target = target[1:end]
			}
		} else if fields := strings.Fields(target); len(fields) > 0 {
			target = fields[0]
		}
		if target != "" {
			targets = append(targets, target)
		}
	}
	return targets
}
