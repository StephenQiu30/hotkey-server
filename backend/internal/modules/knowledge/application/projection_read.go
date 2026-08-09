package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
)

// DocumentProjectionQueryDTO contains only immutable artifact identity and
// integrity facts. Callers cannot provide a Vault path; Knowledge derives it
// from the document identity and transformer profile.
type DocumentProjectionQueryDTO struct {
	DocumentID               int64
	DocumentVersionID        int64
	ArtifactType             string
	TransformerProfileSHA256 string
	SHA256                   string
	SizeBytes                int64
	MaxBytes                 int64
}

// DocumentProjectionResultDTO is a provider-neutral, verified read result.
// It deliberately omits filesystem paths and storage configuration.
type DocumentProjectionResultDTO struct {
	Content   string
	MIMEType  string
	SHA256    string
	SizeBytes int64
}

// DocumentProjectionReader is Knowledge's narrow read-only Application port.
type DocumentProjectionReader interface {
	ReadDocumentProjection(context.Context, DocumentProjectionQueryDTO) (DocumentProjectionResultDTO, error)
}

var _ DocumentProjectionReader = (*ProjectionService)(nil)

// ReadDocumentProjection derives the immutable receipt, delegates the bounded
// read to the Vault boundary, and independently verifies the returned bytes.
func (service *ProjectionService) ReadDocumentProjection(ctx context.Context, query DocumentProjectionQueryDTO) (DocumentProjectionResultDTO, error) {
	format := domain.ProjectionFormat(query.ArtifactType)
	receipt := domain.ProjectionReceipt{
		DocumentID: query.DocumentID, DocumentVersionID: query.DocumentVersionID, Format: format,
		TransformerProfileSHA256: query.TransformerProfileSHA256,
		RelativePath:             domain.ProjectionRelativePath(query.DocumentID, query.DocumentVersionID, format, query.TransformerProfileSHA256),
		SHA256:                   query.SHA256, SizeBytes: query.SizeBytes,
	}
	if err := receipt.Validate(); err != nil {
		return DocumentProjectionResultDTO{}, fmt.Errorf("invalid document projection receipt: %w", err)
	}
	if query.MaxBytes <= 0 || query.SizeBytes > query.MaxBytes {
		return DocumentProjectionResultDTO{}, fmt.Errorf("document projection read limit is invalid")
	}
	content, err := service.Read(ctx, ReadProjectionQuery{
		DocumentID: query.DocumentID, DocumentVersionID: query.DocumentVersionID,
		Format: projectionApplicationFormat(format), TransformerProfileSHA256: query.TransformerProfileSHA256,
		RelativePath: receipt.RelativePath, SHA256: query.SHA256, SizeBytes: query.SizeBytes, MaxBytes: query.MaxBytes,
	})
	if err != nil {
		return DocumentProjectionResultDTO{}, err
	}
	digest := sha256.Sum256(content.Content)
	if content.MIMEType != format.MIMEType() || content.SHA256 != receipt.SHA256 ||
		content.SizeBytes != receipt.SizeBytes || int64(len(content.Content)) != receipt.SizeBytes ||
		hex.EncodeToString(digest[:]) != receipt.SHA256 || !utf8.Valid(content.Content) {
		return DocumentProjectionResultDTO{}, ErrProjectionIntegrity
	}
	return DocumentProjectionResultDTO{
		Content: string(content.Content), MIMEType: content.MIMEType,
		SHA256: content.SHA256, SizeBytes: content.SizeBytes,
	}, nil
}
