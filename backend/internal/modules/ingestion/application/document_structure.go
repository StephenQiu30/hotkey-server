package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"golang.org/x/text/unicode/norm"
)

const (
	// CanonicalDocumentStructureProfileVersion is deliberately explicit and
	// local-only. Changing its lexicons, tokenization, bounds or canonical keys
	// requires a new version and therefore a new search-projection identity.
	CanonicalDocumentStructureProfileVersion = "document-structure-lexicon-v2"
	MaximumDocumentStructureKeys             = 512
)

// ExtractDocumentStructureCommand carries canonical in-memory plaintext. It
// must never be persisted as a job argument or infrastructure Record.
type ExtractDocumentStructureCommand struct {
	DocumentVersionID int64
	ContentSHA256     string
	Title             string
	Plaintext         string
	Language          string
}

// ExtractDocumentStructureResult contains only bounded normalized recall
// keys. They are matching aids, not claims that an entity or location is true.
type ExtractDocumentStructureResult struct {
	DocumentVersionID int64
	ContentSHA256     string
	ProfileVersion    string
	EntityKeys        []string
	ActionKeys        []string
	LocationKeys      []string
	RegionKeys        []string
}

type DocumentStructureExtractor interface {
	ExtractDocumentStructure(context.Context, ExtractDocumentStructureCommand) (ExtractDocumentStructureResult, error)
}

func validateDocumentStructureCommand(command ExtractDocumentStructureCommand) error {
	if command.DocumentVersionID <= 0 || !validLowerHexSHA256(command.ContentSHA256) ||
		strings.TrimSpace(command.Title) == "" || len(command.Title) > 16<<10 || !utf8.ValidString(command.Title) ||
		command.Plaintext == "" || len(command.Plaintext) > MaximumCanonicalSourceBodyBytes || !utf8.ValidString(command.Plaintext) ||
		strings.ContainsRune(command.Plaintext, '\r') || norm.NFC.String(command.Plaintext) != command.Plaintext ||
		strings.TrimSpace(command.Language) != command.Language || command.Language == "" || len(command.Language) > 64 || !utf8.ValidString(command.Language) {
		return fmt.Errorf("%w: document structure input is invalid", sharedrepository.ErrInvalidInput)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(command.Plaintext)))
	if digest != command.ContentSHA256 {
		return fmt.Errorf("%w: document structure plaintext digest changed", sharedrepository.ErrConflict)
	}
	return nil
}

func validateDocumentStructureResult(result ExtractDocumentStructureResult, command ExtractDocumentStructureCommand) error {
	if result.DocumentVersionID != command.DocumentVersionID || result.ContentSHA256 != command.ContentSHA256 ||
		result.ProfileVersion != CanonicalDocumentStructureProfileVersion {
		return fmt.Errorf("%w: document structure identity changed", sharedrepository.ErrConflict)
	}
	for _, values := range [][]string{result.EntityKeys, result.ActionKeys, result.LocationKeys, result.RegionKeys} {
		if len(values) > MaximumDocumentStructureKeys {
			return fmt.Errorf("%w: document structure output exceeds its bound", sharedrepository.ErrConflict)
		}
		normalized, err := normalizedRecallProjectionKeys(values)
		if err != nil || !reflect.DeepEqual(normalized, values) {
			return fmt.Errorf("%w: document structure output is not canonical", sharedrepository.ErrConflict)
		}
	}
	return nil
}

func cloneDocumentStructureKeys(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
