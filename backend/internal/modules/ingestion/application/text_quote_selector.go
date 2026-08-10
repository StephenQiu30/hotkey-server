package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const CanonicalDocumentTextQuoteSelectorVersion = ingestiondomain.CanonicalTextQuoteSelectorVersion

type CreateTextQuoteSelectorCommand struct {
	DocumentVersionID    int64
	ExactQuote           string
	Prefix               string
	Suffix               string
	UTF8ByteStart        int64
	UTF8ByteEnd          int64
	PlaintextSHA256      string
	NormalizationVersion string
	DecisionAt           time.Time
}

type CreateTextQuoteSelectorResult struct{ Selector TextQuoteSelectorDTO }

// LocateTextQuoteSelectorCommand lets a human reviewer submit an exact excerpt
// without calculating UTF-8 byte offsets in a Markdown renderer. The service
// accepts it only when the excerpt occurs exactly once in the immutable NFC
// plaintext projection; all offsets and W3C context are derived server-side.
type LocateTextQuoteSelectorCommand struct {
	DocumentVersionID    int64
	ExactQuote           string
	PlaintextSHA256      string
	NormalizationVersion string
	DecisionAt           time.Time
}

type TextQuoteProjectionArtifactDTO struct {
	ID                       int64
	ArtifactType             string
	TransformerProfileSHA256 string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
	RetentionUntil           time.Time
}

type TextQuoteAnchorBlockDTO struct {
	Ordinal                int
	PlaintextUTF8ByteStart int64
	PlaintextUTF8ByteEnd   int64
	MarkdownAnchor         string
}

type TextQuoteSelectorTargetQuery struct {
	DocumentVersionID int64
	PlaintextSHA256   string
	DecisionAt        time.Time
}

type TextQuoteSelectorTargetDTO struct {
	SourceConnectionID     int64
	DocumentID             int64
	DocumentVersionID      int64
	ContentSHA256          string
	DocumentLifecycleState string
	PlaintextArtifact      TextQuoteProjectionArtifactDTO
	MarkdownArtifactID     int64
	AnchorMapSHA256        string
	AnchorBlocks           []TextQuoteAnchorBlockDTO
	QuoteRightsDecisionID  int64
	RetainRightsDecisionID int64
	RetentionUntil         time.Time
	DecisionAt             time.Time
}

type PersistTextQuoteSelectorCommand struct {
	SourceConnectionID     int64
	DocumentVersionID      int64
	PlaintextArtifactID    int64
	MarkdownArtifactID     int64
	QuoteRightsDecisionID  int64
	RetainRightsDecisionID int64
	ExactQuote             string
	Prefix                 string
	Suffix                 string
	UTF8ByteStart          int64
	UTF8ByteEnd            int64
	QuoteSHA256            string
	PlaintextSHA256        string
	NormalizationVersion   string
	SelectorVersion        string
	AnchorMapSHA256        string
	MarkdownAnchor         *string
	RetentionUntil         time.Time
	DecisionAt             time.Time
}

type TextQuoteSelectorDTO struct {
	ID                     int64
	Version                int64
	SourceConnectionID     int64
	DocumentVersionID      int64
	PlaintextArtifactID    int64
	MarkdownArtifactID     int64
	QuoteRightsDecisionID  int64
	RetainRightsDecisionID int64
	ExactQuote             string
	Prefix                 string
	Suffix                 string
	UTF8ByteStart          int64
	UTF8ByteEnd            int64
	QuoteSHA256            string
	PlaintextSHA256        string
	NormalizationVersion   string
	SelectorVersion        string
	AnchorMapSHA256        string
	MarkdownAnchor         *string
	RetentionUntil         time.Time
	CreatedAt              time.Time
}

type TextQuoteSelectorRepository interface {
	ReadTextQuoteSelectorTarget(context.Context, TextQuoteSelectorTargetQuery) (TextQuoteSelectorTargetDTO, error)
	PersistTextQuoteSelector(context.Context, PersistTextQuoteSelectorCommand) (TextQuoteSelectorDTO, error)
}

type TextQuoteSelectorDependencies struct {
	Repository  TextQuoteSelectorRepository
	Projections knowledgeapplication.DocumentProjectionReader
}

type TextQuoteSelectorService struct {
	repository  TextQuoteSelectorRepository
	projections knowledgeapplication.DocumentProjectionReader
}

func NewTextQuoteSelectorService(dependencies TextQuoteSelectorDependencies) (*TextQuoteSelectorService, error) {
	if dependencies.Repository == nil || dependencies.Projections == nil {
		return nil, errors.New("text quote selector dependencies are required")
	}
	return &TextQuoteSelectorService{repository: dependencies.Repository, projections: dependencies.Projections}, nil
}

func (service *TextQuoteSelectorService) Create(ctx context.Context, command CreateTextQuoteSelectorCommand) (CreateTextQuoteSelectorResult, error) {
	if service == nil || service.repository == nil || service.projections == nil || command.DocumentVersionID <= 0 ||
		command.DecisionAt.IsZero() || !validLowerHexSHA256(command.PlaintextSHA256) ||
		command.NormalizationVersion != CanonicalDocumentTextNormalizationVersion {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: invalid text quote selector command", sharedrepository.ErrInvalidInput)
	}
	target, projection, err := service.readTextQuoteProjection(ctx, command.DocumentVersionID, command.PlaintextSHA256, command.DecisionAt)
	if err != nil {
		return CreateTextQuoteSelectorResult{}, err
	}
	if err := validateTextQuoteTarget(target, command); err != nil {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConflict, err)
	}
	return service.persistTextQuoteSelector(ctx, command, target, projection.Content)
}

func (service *TextQuoteSelectorService) LocateAndCreate(ctx context.Context, command LocateTextQuoteSelectorCommand) (CreateTextQuoteSelectorResult, error) {
	if service == nil || service.repository == nil || service.projections == nil || command.DocumentVersionID <= 0 ||
		command.DecisionAt.IsZero() || command.ExactQuote == "" || len(command.ExactQuote) > ingestiondomain.MaximumExactQuoteBytes ||
		!validLowerHexSHA256(command.PlaintextSHA256) || command.NormalizationVersion != CanonicalDocumentTextNormalizationVersion {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: invalid located text quote selector command", sharedrepository.ErrInvalidInput)
	}
	target, projection, err := service.readTextQuoteProjection(ctx, command.DocumentVersionID, command.PlaintextSHA256, command.DecisionAt)
	if err != nil {
		return CreateTextQuoteSelectorResult{}, err
	}
	start := strings.Index(projection.Content, command.ExactQuote)
	if start < 0 || strings.Index(projection.Content[start+len(command.ExactQuote):], command.ExactQuote) >= 0 {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: exact quote must occur exactly once in the immutable plaintext", sharedrepository.ErrInvalidInput)
	}
	end := start + len(command.ExactQuote)
	prefix, suffix := canonicalTextQuoteContext(string(projection.Content), int64(start), int64(end))
	located := CreateTextQuoteSelectorCommand{
		DocumentVersionID: command.DocumentVersionID, ExactQuote: command.ExactQuote,
		Prefix: prefix, Suffix: suffix, UTF8ByteStart: int64(start), UTF8ByteEnd: int64(end),
		PlaintextSHA256: command.PlaintextSHA256, NormalizationVersion: command.NormalizationVersion,
		DecisionAt: command.DecisionAt.UTC(),
	}
	if err := validateTextQuoteTarget(target, located); err != nil {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConflict, err)
	}
	return service.persistTextQuoteSelector(ctx, located, target, projection.Content)
}

func (service *TextQuoteSelectorService) readTextQuoteProjection(ctx context.Context, documentVersionID int64, plaintextSHA256 string, decisionAt time.Time) (TextQuoteSelectorTargetDTO, knowledgeapplication.DocumentProjectionResultDTO, error) {
	target, err := service.repository.ReadTextQuoteSelectorTarget(ctx, TextQuoteSelectorTargetQuery{
		DocumentVersionID: documentVersionID, PlaintextSHA256: plaintextSHA256, DecisionAt: decisionAt.UTC(),
	})
	if err != nil {
		return TextQuoteSelectorTargetDTO{}, knowledgeapplication.DocumentProjectionResultDTO{}, fmt.Errorf("read text quote selector target: %w", err)
	}
	projection, err := service.projections.ReadDocumentProjection(ctx, knowledgeapplication.DocumentProjectionQueryDTO{
		DocumentID: target.DocumentID, DocumentVersionID: target.DocumentVersionID,
		ArtifactType: target.PlaintextArtifact.ArtifactType, TransformerProfileSHA256: target.PlaintextArtifact.TransformerProfileSHA256,
		SHA256: target.PlaintextArtifact.SHA256, SizeBytes: target.PlaintextArtifact.SizeBytes, MaxBytes: MaximumCanonicalSourceBodyBytes,
	})
	if err != nil {
		return TextQuoteSelectorTargetDTO{}, knowledgeapplication.DocumentProjectionResultDTO{}, fmt.Errorf("read immutable plaintext projection: %w", err)
	}
	if projection.MIMEType != target.PlaintextArtifact.MIMEType || projection.SHA256 != target.PlaintextArtifact.SHA256 ||
		projection.SizeBytes != target.PlaintextArtifact.SizeBytes || int64(len(projection.Content)) != projection.SizeBytes {
		return TextQuoteSelectorTargetDTO{}, knowledgeapplication.DocumentProjectionResultDTO{}, fmt.Errorf("%w: plaintext projection receipt changed", sharedrepository.ErrConflict)
	}
	return target, projection, nil
}

func (service *TextQuoteSelectorService) persistTextQuoteSelector(ctx context.Context, command CreateTextQuoteSelectorCommand, target TextQuoteSelectorTargetDTO, plaintext string) (CreateTextQuoteSelectorResult, error) {
	selector, err := ingestiondomain.BuildTextQuoteSelector(plaintext, ingestiondomain.TextQuoteSelectorCandidate{
		ExactQuote: command.ExactQuote, Prefix: command.Prefix, Suffix: command.Suffix,
		UTF8ByteStart: command.UTF8ByteStart, UTF8ByteEnd: command.UTF8ByteEnd,
		PlaintextSHA256: command.PlaintextSHA256, NormalizationVersion: command.NormalizationVersion,
	})
	if err != nil {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	anchor := textQuoteMarkdownAnchor(target.AnchorBlocks, selector.UTF8ByteStart, selector.UTF8ByteEnd)
	persisted, err := service.repository.PersistTextQuoteSelector(ctx, PersistTextQuoteSelectorCommand{
		SourceConnectionID: target.SourceConnectionID, DocumentVersionID: target.DocumentVersionID,
		PlaintextArtifactID: target.PlaintextArtifact.ID, MarkdownArtifactID: target.MarkdownArtifactID,
		QuoteRightsDecisionID: target.QuoteRightsDecisionID, RetainRightsDecisionID: target.RetainRightsDecisionID,
		ExactQuote: selector.ExactQuote, Prefix: selector.Prefix, Suffix: selector.Suffix,
		UTF8ByteStart: selector.UTF8ByteStart, UTF8ByteEnd: selector.UTF8ByteEnd, QuoteSHA256: selector.QuoteSHA256,
		PlaintextSHA256: selector.PlaintextSHA256, NormalizationVersion: selector.NormalizationVersion,
		SelectorVersion: selector.SelectorVersion, AnchorMapSHA256: target.AnchorMapSHA256, MarkdownAnchor: anchor,
		RetentionUntil: target.RetentionUntil, DecisionAt: command.DecisionAt.UTC(),
	})
	if err != nil {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("persist immutable text quote selector: %w", err)
	}
	if !textQuoteSelectorReceiptMatches(persisted, target, selector, anchor) {
		return CreateTextQuoteSelectorResult{}, fmt.Errorf("%w: text quote selector receipt changed", sharedrepository.ErrConflict)
	}
	return CreateTextQuoteSelectorResult{Selector: persisted}, nil
}

func validateTextQuoteTarget(target TextQuoteSelectorTargetDTO, command CreateTextQuoteSelectorCommand) error {
	artifact := target.PlaintextArtifact
	if target.SourceConnectionID <= 0 || target.DocumentID <= 0 || target.DocumentVersionID != command.DocumentVersionID ||
		target.ContentSHA256 != command.PlaintextSHA256 || target.DocumentLifecycleState != DocumentReadable ||
		artifact.ID <= 0 || artifact.ArtifactType != DocumentProjectionPlaintext || artifact.SHA256 != command.PlaintextSHA256 ||
		!validLowerHexSHA256(artifact.TransformerProfileSHA256) || artifact.MIMEType != "text/plain; charset=utf-8" ||
		artifact.SizeBytes <= 0 || !artifact.RetentionUntil.After(command.DecisionAt) || !target.RetentionUntil.After(command.DecisionAt) ||
		target.RetentionUntil.After(artifact.RetentionUntil) || target.QuoteRightsDecisionID <= 0 ||
		target.RetainRightsDecisionID <= 0 || !target.DecisionAt.Equal(command.DecisionAt) {
		return errors.New("text quote selector target is not currently authorized and readable")
	}
	if target.MarkdownArtifactID <= 0 || !validLowerHexSHA256(target.AnchorMapSHA256) || len(target.AnchorBlocks) == 0 {
		return errors.New("text quote selector target has no exact Markdown anchor map")
	}
	return nil
}

func textQuoteMarkdownAnchor(blocks []TextQuoteAnchorBlockDTO, start, end int64) *string {
	for _, block := range blocks {
		if block.PlaintextUTF8ByteStart <= start && end <= block.PlaintextUTF8ByteEnd && block.MarkdownAnchor != "" {
			anchor := block.MarkdownAnchor
			return &anchor
		}
	}
	return nil
}

func textQuoteSelectorReceiptMatches(value TextQuoteSelectorDTO, target TextQuoteSelectorTargetDTO, selector ingestiondomain.TextQuoteSelector, anchor *string) bool {
	if value.ID <= 0 || value.Version <= 0 || value.SourceConnectionID != target.SourceConnectionID || value.DocumentVersionID != target.DocumentVersionID ||
		value.PlaintextArtifactID != target.PlaintextArtifact.ID || value.MarkdownArtifactID != target.MarkdownArtifactID ||
		value.QuoteRightsDecisionID != target.QuoteRightsDecisionID || value.RetainRightsDecisionID != target.RetainRightsDecisionID ||
		value.ExactQuote != selector.ExactQuote || value.Prefix != selector.Prefix || value.Suffix != selector.Suffix ||
		value.UTF8ByteStart != selector.UTF8ByteStart || value.UTF8ByteEnd != selector.UTF8ByteEnd ||
		value.QuoteSHA256 != selector.QuoteSHA256 || value.PlaintextSHA256 != selector.PlaintextSHA256 ||
		value.NormalizationVersion != selector.NormalizationVersion || value.SelectorVersion != selector.SelectorVersion ||
		value.AnchorMapSHA256 != target.AnchorMapSHA256 || !value.RetentionUntil.Equal(target.RetentionUntil) || value.CreatedAt.IsZero() {
		return false
	}
	return optionalQuoteStringEqual(value.MarkdownAnchor, anchor)
}

func optionalQuoteStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func canonicalTextQuoteContext(plaintext string, start, end int64) (string, string) {
	return ingestiondomain.CanonicalTextQuoteContext(plaintext, start, end)
}
