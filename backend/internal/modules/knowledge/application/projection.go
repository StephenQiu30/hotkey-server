package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
)

var (
	ErrProjectionInvalid     = errors.New("immutable projection request is invalid")
	ErrProjectionConflict    = errors.New("immutable projection content conflict")
	ErrProjectionIntegrity   = errors.New("immutable projection integrity failure")
	ErrProjectionTooLarge    = errors.New("immutable projection exceeds read limit")
	ErrProjectionNotFound    = errors.New("immutable projection is missing")
	ErrProjectionUnavailable = errors.New("immutable projection store unavailable")
)

type ProjectionFormat string

const (
	ProjectionFormatMarkdown  ProjectionFormat = "markdown"
	ProjectionFormatPlaintext ProjectionFormat = "plaintext"
)

func (format ProjectionFormat) valid() bool {
	return format == ProjectionFormatMarkdown || format == ProjectionFormatPlaintext
}

type PublishProjectionCommand struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   ProjectionFormat
	TransformerProfileSHA256 string
	Content                  []byte
	SHA256                   string
}

type PublishProjectionResult struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   ProjectionFormat
	TransformerProfileSHA256 string
	RelativePath             string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
}

type ReadProjectionQuery struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   ProjectionFormat
	TransformerProfileSHA256 string
	RelativePath             string
	SHA256                   string
	SizeBytes                int64
	MaxBytes                 int64
}

type ReadProjectionResult struct {
	Content   []byte
	MIMEType  string
	SHA256    string
	SizeBytes int64
}

// StoreProjectionCommand is the provider-neutral immutable write contract.
// Application derives RelativePath and MIMEType after Domain validation;
// infrastructure cannot choose another location or interpretation.
type StoreProjectionCommand struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   string
	TransformerProfileSHA256 string
	RelativePath             string
	MIMEType                 string
	Content                  []byte
	SHA256                   string
}

type ProjectionStoreReceiptDTO struct {
	DocumentID               int64
	DocumentVersionID        int64
	Format                   string
	TransformerProfileSHA256 string
	RelativePath             string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
}

type ReadStoredProjectionCommand struct {
	Receipt  ProjectionStoreReceiptDTO
	MaxBytes int64
}

type StoredProjectionContentDTO struct {
	Content   []byte
	MIMEType  string
	SHA256    string
	SizeBytes int64
}

type ProjectionPublisher interface {
	Publish(context.Context, PublishProjectionCommand) (PublishProjectionResult, error)
}

// ProjectionStore is Knowledge's immutable Vault boundary. Provider paths and
// filesystem details stay behind this interface.
type ProjectionStore interface {
	PutIfAbsent(context.Context, StoreProjectionCommand) (ProjectionStoreReceiptDTO, error)
	ReadProjection(context.Context, ReadStoredProjectionCommand) (StoredProjectionContentDTO, error)
}

// ProjectionService is the only Application contract other modules use for
// automatic document projections. It is deliberately separate from the
// human-reviewed Knowledge proposal workflow.
type ProjectionService struct {
	store ProjectionStore
}

func NewProjectionService(store ProjectionStore) (*ProjectionService, error) {
	if store == nil {
		return nil, errors.New("knowledge projection store is required")
	}
	return &ProjectionService{store: store}, nil
}

func (service *ProjectionService) Publish(ctx context.Context, command PublishProjectionCommand) (PublishProjectionResult, error) {
	if service == nil || service.store == nil {
		return PublishProjectionResult{}, errors.New("knowledge projection service is not initialized")
	}
	projection := immutableProjectionFromCommand(command)
	if err := projection.Validate(); err != nil {
		if errors.Is(err, domain.ErrProjectionIntegrity) {
			return PublishProjectionResult{}, ErrProjectionIntegrity
		}
		return PublishProjectionResult{}, ErrProjectionInvalid
	}
	storeCommand := projectionStoreCommandFromDomain(projection)
	receiptDTO, err := service.store.PutIfAbsent(ctx, storeCommand)
	if err != nil {
		return PublishProjectionResult{}, mapProjectionError(err)
	}
	receipt, err := projectionReceiptDomainFromDTO(receiptDTO)
	if err != nil {
		return PublishProjectionResult{}, fmt.Errorf("%w: invalid Vault receipt", ErrProjectionIntegrity)
	}
	if err := receipt.Validate(); err != nil {
		return PublishProjectionResult{}, fmt.Errorf("%w: invalid Vault receipt", ErrProjectionIntegrity)
	}
	if receipt.DocumentID != projection.DocumentID || receipt.DocumentVersionID != projection.DocumentVersionID ||
		receipt.Format != projection.Format || receipt.TransformerProfileSHA256 != projection.TransformerProfileSHA256 ||
		receipt.RelativePath != projection.RelativePath() || receipt.SHA256 != projection.SHA256 ||
		receipt.SizeBytes != int64(len(projection.Content)) {
		return PublishProjectionResult{}, ErrProjectionIntegrity
	}
	return publishProjectionResultFromReceipt(receipt), nil
}

func (service *ProjectionService) Read(ctx context.Context, query ReadProjectionQuery) (ReadProjectionResult, error) {
	if service == nil || service.store == nil {
		return ReadProjectionResult{}, errors.New("knowledge projection service is not initialized")
	}
	receipt := projectionReceiptFromQuery(query)
	if err := receipt.Validate(); err != nil {
		return ReadProjectionResult{}, ErrProjectionInvalid
	}
	if query.MaxBytes <= 0 {
		return ReadProjectionResult{}, ErrProjectionInvalid
	}
	contentDTO, err := service.store.ReadProjection(ctx, ReadStoredProjectionCommand{
		Receipt: projectionReceiptDTOFromDomain(receipt), MaxBytes: query.MaxBytes,
	})
	if err != nil {
		return ReadProjectionResult{}, mapProjectionError(err)
	}
	content := projectionContentDomainFromDTO(contentDTO)
	digest := sha256.Sum256(content.Content)
	if content.SHA256 != receipt.SHA256 || content.SizeBytes != receipt.SizeBytes ||
		int64(len(content.Content)) != receipt.SizeBytes || hex.EncodeToString(digest[:]) != receipt.SHA256 ||
		content.MIMEType != receipt.Format.MIMEType() {
		return ReadProjectionResult{}, ErrProjectionIntegrity
	}
	return ReadProjectionResult{
		Content: append([]byte(nil), content.Content...), MIMEType: content.MIMEType,
		SHA256: content.SHA256, SizeBytes: content.SizeBytes,
	}, nil
}

func ValidateStoreProjectionCommand(command StoreProjectionCommand) error {
	projection := domain.ImmutableProjection{
		DocumentID: command.DocumentID, DocumentVersionID: command.DocumentVersionID,
		Format: domain.ProjectionFormat(command.Format), TransformerProfileSHA256: command.TransformerProfileSHA256,
		Content: append([]byte(nil), command.Content...), SHA256: command.SHA256,
	}
	if err := projection.Validate(); err != nil || command.RelativePath != projection.RelativePath() ||
		command.MIMEType != projection.MIMEType() {
		return ErrProjectionInvalid
	}
	return nil
}

func ValidateProjectionStoreReceiptDTO(receipt ProjectionStoreReceiptDTO) error {
	_, err := projectionReceiptDomainFromDTO(receipt)
	return err
}

func ValidateReadStoredProjectionCommand(command ReadStoredProjectionCommand) error {
	if command.MaxBytes <= 0 || command.Receipt.SizeBytes > command.MaxBytes {
		return ErrProjectionInvalid
	}
	return ValidateProjectionStoreReceiptDTO(command.Receipt)
}

func immutableProjectionFromCommand(command PublishProjectionCommand) domain.ImmutableProjection {
	return domain.ImmutableProjection{
		DocumentID: command.DocumentID, DocumentVersionID: command.DocumentVersionID,
		Format: projectionDomainFormat(command.Format), TransformerProfileSHA256: command.TransformerProfileSHA256,
		Content: append([]byte(nil), command.Content...), SHA256: command.SHA256,
	}
}

func projectionStoreCommandFromDomain(projection domain.ImmutableProjection) StoreProjectionCommand {
	return StoreProjectionCommand{
		DocumentID: projection.DocumentID, DocumentVersionID: projection.DocumentVersionID,
		Format: string(projection.Format), TransformerProfileSHA256: projection.TransformerProfileSHA256,
		RelativePath: projection.RelativePath(), MIMEType: projection.MIMEType(),
		Content: append([]byte(nil), projection.Content...), SHA256: projection.SHA256,
	}
}

func projectionReceiptFromQuery(query ReadProjectionQuery) domain.ProjectionReceipt {
	return domain.ProjectionReceipt{
		DocumentID: query.DocumentID, DocumentVersionID: query.DocumentVersionID,
		Format: projectionDomainFormat(query.Format), TransformerProfileSHA256: query.TransformerProfileSHA256,
		RelativePath: query.RelativePath, SHA256: query.SHA256, SizeBytes: query.SizeBytes,
	}
}

func projectionReceiptDTOFromDomain(receipt domain.ProjectionReceipt) ProjectionStoreReceiptDTO {
	return ProjectionStoreReceiptDTO{
		DocumentID: receipt.DocumentID, DocumentVersionID: receipt.DocumentVersionID,
		Format: string(receipt.Format), TransformerProfileSHA256: receipt.TransformerProfileSHA256,
		RelativePath: receipt.RelativePath, MIMEType: receipt.Format.MIMEType(),
		SHA256: receipt.SHA256, SizeBytes: receipt.SizeBytes,
	}
}

func projectionReceiptDomainFromDTO(receipt ProjectionStoreReceiptDTO) (domain.ProjectionReceipt, error) {
	result := domain.ProjectionReceipt{
		DocumentID: receipt.DocumentID, DocumentVersionID: receipt.DocumentVersionID,
		Format: domain.ProjectionFormat(receipt.Format), TransformerProfileSHA256: receipt.TransformerProfileSHA256,
		RelativePath: receipt.RelativePath, SHA256: receipt.SHA256, SizeBytes: receipt.SizeBytes,
	}
	if err := result.Validate(); err != nil || receipt.MIMEType != result.Format.MIMEType() {
		return domain.ProjectionReceipt{}, ErrProjectionIntegrity
	}
	return result, nil
}

func projectionContentDomainFromDTO(content StoredProjectionContentDTO) domain.ProjectionContent {
	return domain.ProjectionContent{
		Content: append([]byte(nil), content.Content...), MIMEType: content.MIMEType,
		SHA256: content.SHA256, SizeBytes: content.SizeBytes,
	}
}

func publishProjectionResultFromReceipt(receipt domain.ProjectionReceipt) PublishProjectionResult {
	return PublishProjectionResult{
		DocumentID: receipt.DocumentID, DocumentVersionID: receipt.DocumentVersionID,
		Format: projectionApplicationFormat(receipt.Format), TransformerProfileSHA256: receipt.TransformerProfileSHA256,
		RelativePath: receipt.RelativePath, MIMEType: receipt.Format.MIMEType(),
		SHA256: receipt.SHA256, SizeBytes: receipt.SizeBytes,
	}
}

func projectionDomainFormat(format ProjectionFormat) domain.ProjectionFormat {
	switch format {
	case ProjectionFormatMarkdown:
		return domain.ProjectionMarkdown
	case ProjectionFormatPlaintext:
		return domain.ProjectionPlaintext
	default:
		return ""
	}
}

func projectionApplicationFormat(format domain.ProjectionFormat) ProjectionFormat {
	switch format {
	case domain.ProjectionMarkdown:
		return ProjectionFormatMarkdown
	case domain.ProjectionPlaintext:
		return ProjectionFormatPlaintext
	default:
		return ""
	}
}

func mapProjectionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrProjectionConflict):
		return ErrProjectionConflict
	case errors.Is(err, ErrProjectionIntegrity):
		return ErrProjectionIntegrity
	case errors.Is(err, ErrProjectionTooLarge):
		return ErrProjectionTooLarge
	case errors.Is(err, ErrProjectionNotFound):
		return ErrProjectionNotFound
	case errors.Is(err, domain.ErrProjectionConflict):
		return ErrProjectionConflict
	case errors.Is(err, domain.ErrProjectionIntegrity):
		return ErrProjectionIntegrity
	case errors.Is(err, domain.ErrProjectionTooLarge):
		return ErrProjectionTooLarge
	case errors.Is(err, fs.ErrNotExist):
		return ErrProjectionNotFound
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrProjectionUnavailable
	}
}
