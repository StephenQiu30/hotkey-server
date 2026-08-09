package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
)

var (
	ErrProjectionConflict  = errors.New("immutable projection already exists with different content")
	ErrProjectionIntegrity = errors.New("immutable projection integrity check failed")
	ErrProjectionTooLarge  = errors.New("immutable projection exceeds read limit")
)

type ProjectionFormat string

const (
	ProjectionMarkdown  ProjectionFormat = "markdown"
	ProjectionPlaintext ProjectionFormat = "plaintext"
)

func (format ProjectionFormat) Valid() bool {
	return format == ProjectionMarkdown || format == ProjectionPlaintext
}

func (format ProjectionFormat) extension() string {
	if format == ProjectionMarkdown {
		return "md"
	}
	if format == ProjectionPlaintext {
		return "txt"
	}
	return ""
}

func (format ProjectionFormat) MIMEType() string {
	if format == ProjectionMarkdown {
		return "text/markdown; charset=utf-8"
	}
	if format == ProjectionPlaintext {
		return "text/plain; charset=utf-8"
	}
	return ""
}

// ImmutableProjection is a derived, human-readable representation of one
// immutable document version. Its path is derived only from database IDs;
// callers cannot choose a filesystem path.
type ImmutableProjection struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   ProjectionFormat
	TransformerProfileSHA256 string
	Content                  []byte
	SHA256                   string
}

func (projection ImmutableProjection) Validate() error {
	if projection.DocumentID <= 0 || projection.DocumentVersionID <= 0 {
		return fmt.Errorf("projection document and version are required")
	}
	if !projection.Format.Valid() {
		return fmt.Errorf("projection format is invalid")
	}
	if !validProjectionSHA256(projection.TransformerProfileSHA256) {
		return fmt.Errorf("projection transformer profile SHA-256 is invalid")
	}
	if len(projection.Content) == 0 {
		return fmt.Errorf("projection content is required")
	}
	if !validProjectionSHA256(projection.SHA256) {
		return fmt.Errorf("projection SHA-256 is invalid")
	}
	digest := sha256.Sum256(projection.Content)
	if hex.EncodeToString(digest[:]) != projection.SHA256 {
		return fmt.Errorf("%w: declared SHA-256 does not match content", ErrProjectionIntegrity)
	}
	return nil
}

func (projection ImmutableProjection) RelativePath() string {
	return ProjectionRelativePath(projection.DocumentID, projection.DocumentVersionID, projection.Format, projection.TransformerProfileSHA256)
}

func (projection ImmutableProjection) MIMEType() string { return projection.Format.MIMEType() }

type ProjectionReceipt struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   ProjectionFormat
	TransformerProfileSHA256 string
	RelativePath             string
	SHA256                   string
	SizeBytes                int64
}

func (receipt ProjectionReceipt) Validate() error {
	if receipt.DocumentID <= 0 || receipt.DocumentVersionID <= 0 || !receipt.Format.Valid() {
		return fmt.Errorf("projection receipt identity is invalid")
	}
	if !validProjectionSHA256(receipt.TransformerProfileSHA256) || receipt.RelativePath != ProjectionRelativePath(receipt.DocumentID, receipt.DocumentVersionID, receipt.Format, receipt.TransformerProfileSHA256) {
		return fmt.Errorf("projection receipt path is invalid")
	}
	if !validProjectionSHA256(receipt.SHA256) || receipt.SizeBytes <= 0 {
		return fmt.Errorf("projection receipt integrity metadata is invalid")
	}
	return nil
}

type ProjectionContent struct {
	Content   []byte
	MIMEType  string
	SHA256    string
	SizeBytes int64
}

func ProjectionRelativePath(documentID, documentVersionID int64, format ProjectionFormat, transformerProfileSHA256 string) string {
	if documentID <= 0 || documentVersionID <= 0 || !format.Valid() || !validProjectionSHA256(transformerProfileSHA256) {
		return ""
	}
	return path.Join(
		"documents", fmt.Sprintf("%d", documentID), fmt.Sprintf("%d", documentVersionID), string(format),
		transformerProfileSHA256+"."+format.extension(),
	)
}

func validProjectionSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
