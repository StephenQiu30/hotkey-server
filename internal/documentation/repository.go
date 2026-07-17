package docgovernance

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var docNoPattern = regexp.MustCompile(`^[0-9]{3}$`)

var requiredFields = []string{
	"layer", "doc_no", "audience", "feature_area", "purpose", "canonical_path", "status", "version", "owner", "inputs", "outputs", "triggers", "downstream",
}

var validLayers = map[string]bool{
	"Design": true, "PRD": true, "Plan": true, "Acceptance": true, "Operations": true,
}

var validStatuses = map[string]bool{
	"draft": true, "review": true, "accepted": true, "archived": true,
}

var validAudience = map[string]bool{
	"PM": true, "Dev": true, "QA": true, "Ops": true,
}

// LoadRepository reads every formal Markdown document beneath docs. It
// returns all parsing and structural issues rather than failing fast, so one
// command can show the full set of documents that require attention.
func LoadRepository(root string, _ CommitResolver) (*Repository, []Issue) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, []Issue{{Path: root, Rule: "repository.root", Message: "resolve repository root"}}
	}
	absRoot = filepath.Clean(absRoot)
	repository := &Repository{Root: absRoot}
	issues := []Issue{}
	docsRoot := filepath.Join(absRoot, "docs")
	if _, err := os.Stat(docsRoot); err != nil {
		issues = append(issues, Issue{Path: "docs", Rule: "repository.docs", Message: "formal docs directory is required"})
		sortIssues(issues)
		return repository, issues
	}

	_ = filepath.WalkDir(docsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			issues = append(issues, Issue{Path: repositoryPath(absRoot, path), Rule: "document.read", Message: "read document"})
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		relativePath := repositoryPath(absRoot, path)
		contents, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{Path: relativePath, Rule: "document.read", Message: "read document"})
			return nil
		}
		document, fields, body, err := parseDocument(relativePath, contents)
		if err != nil {
			issues = append(issues, Issue{Path: relativePath, Rule: "frontmatter.parse", Message: err.Error()})
			return nil
		}
		issues = append(issues, validateHeader(document, fields)...)
		document.Links, issues = appendLinks(absRoot, document.Path, body, document.Links, issues)
		repository.Documents = append(repository.Documents, document)
		return nil
	})

	issues = append(issues, validateDocumentUniqueness(repository.Documents)...)
	sortDocuments(repository.Documents)
	sortIssues(issues)
	return repository, issues
}

func validateHeader(document Document, fields map[string]any) []Issue {
	issues := []Issue{}
	for _, field := range requiredFields {
		if _, exists := fields[field]; !exists {
			issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.required", Message: field + " is required"})
		}
	}
	if !validLayers[document.Layer] {
		issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.layer", Message: "layer is invalid"})
	}
	if !docNoPattern.MatchString(document.DocNo) {
		issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.doc_no", Message: "doc_no must use three digits"})
	}
	if !validStatuses[document.Status] {
		issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.status", Message: "status is invalid"})
	}
	if len(document.Audience) == 0 {
		issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.audience", Message: "audience is required"})
	}
	for _, audience := range document.Audience {
		if !validAudience[audience] {
			issues = append(issues, Issue{Path: document.Path, Rule: "frontmatter.audience", Message: "audience contains an invalid value"})
			break
		}
	}
	if canonicalPath, valid := cleanRepositoryPath(document.CanonicalPath); !valid || canonicalPath != document.Path {
		issues = append(issues, Issue{Path: document.Path, Rule: "document.canonical_path", Message: "canonical_path must equal the repository path"})
	}
	return issues
}

func appendLinks(root, documentPath, body string, references []Reference, issues []Issue) ([]Reference, []Issue) {
	for _, rawTarget := range parseMarkdownLinkTargets(body) {
		target, external, err := localLinkTarget(rawTarget)
		if err != nil {
			issues = append(issues, Issue{Path: documentPath, Rule: "link.invalid", Message: "link target is invalid"})
			continue
		}
		if external || target == "" {
			continue
		}
		resolved, valid := resolveDocumentLink(root, documentPath, target)
		if !valid {
			issues = append(issues, Issue{Path: documentPath, Rule: "link.outside_root", Message: "local link escapes the repository"})
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(resolved))); err != nil {
			issues = append(issues, Issue{Path: documentPath, Rule: "link.missing", Message: "local link target is missing: " + resolved})
			continue
		}
		references = append(references, Reference{Path: resolved})
	}
	return references, issues
}

func localLinkTarget(rawTarget string) (string, bool, error) {
	parsed, err := url.Parse(rawTarget)
	if err != nil {
		return "", false, err
	}
	if parsed.Scheme != "" || parsed.Host != "" {
		return "", true, nil
	}
	if parsed.Path == "" {
		return "", false, nil
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false, err
	}
	return filepath.ToSlash(path), false, nil
}

func resolveDocumentLink(root, documentPath, target string) (string, bool) {
	if strings.HasPrefix(target, "/") {
		return "", false
	}
	base := filepath.Join(root, filepath.FromSlash(filepath.Dir(documentPath)))
	candidate := filepath.Clean(filepath.Join(base, filepath.FromSlash(target)))
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func validateDocumentUniqueness(documents []Document) []Issue {
	issues := []Issue{}
	docNumbers := map[string]string{}
	canonicalPaths := map[string]string{}
	for _, document := range documents {
		if document.Layer != "" && document.DocNo != "" {
			key := document.Layer + ":" + document.DocNo
			if first, exists := docNumbers[key]; exists {
				issues = append(issues, Issue{Path: document.Path, Rule: "document.doc_no_unique", Message: "duplicates " + first})
			} else {
				docNumbers[key] = document.Path
			}
		}
		if document.CanonicalPath != "" {
			if first, exists := canonicalPaths[document.CanonicalPath]; exists {
				issues = append(issues, Issue{Path: document.Path, Rule: "document.canonical_path_unique", Message: "duplicates " + first})
			} else {
				canonicalPaths[document.CanonicalPath] = document.Path
			}
		}
	}
	return issues
}

func cleanRepositoryPath(value string) (string, bool) {
	if value == "" || filepath.IsAbs(value) {
		return "", false
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(cleaned), true
}

func repositoryPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func sortDocuments(documents []Document) {
	for left := 0; left < len(documents); left++ {
		for right := left + 1; right < len(documents); right++ {
			if documents[right].Path < documents[left].Path {
				documents[left], documents[right] = documents[right], documents[left]
			}
		}
	}
}

func (issue Issue) String() string {
	return fmt.Sprintf("%s: %s: %s", issue.Path, issue.Rule, issue.Message)
}
