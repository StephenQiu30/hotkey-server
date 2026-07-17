// Package documentation loads and validates the repository's formal Markdown
// documents. It intentionally depends only on files and Git metadata so the
// governance gate can run without application services.
package docgovernance

// Document is the normalized frontmatter and local-reference projection of
// one formal Markdown document.
type Document struct {
	Path          string
	Layer         string
	DocNo         string
	Audience      []string
	FeatureArea   string
	Purpose       string
	CanonicalPath string
	Status        string
	Version       string
	Owner         string
	Inputs        []string
	Outputs       []string
	Triggers      []string
	Downstream    []string

	ExecutionStatus string
	ReviewStatus    string
	Result          string
	Links           []Reference
}

// Reference is a resolved local Markdown link. Path is always rooted at the
// repository and never contains an absolute or escaping path.
type Reference struct {
	Path string
}

// Repository is the complete formal-document projection for one repository.
type Repository struct {
	Root      string
	Documents []Document
}

// CommitResolver isolates Git access for lifecycle evidence checks added by
// subsequent governance tasks. Parsing does not call it.
type CommitResolver interface {
	Resolve(revision string) error
}
