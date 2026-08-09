package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

// GenerateSourceDocumentJobArgs is the complete durable worker contract. Raw
// evidence, body text, object keys, rights receipts, and projection bytes are
// always reread by Application services and can never enter this DTO.
type GenerateSourceDocumentJobArgs struct {
	EvidenceReferenceID int64  `json:"evidence_reference_id"`
	TraceID             string `json:"trace_id"`
}

func (args GenerateSourceDocumentJobArgs) validate() error {
	if args.EvidenceReferenceID <= 0 || !validOptionalTraceID(args.TraceID) {
		return fmt.Errorf("source document generation job args are invalid")
	}
	return nil
}

type sourceDocumentGenerator interface {
	Generate(context.Context, ingestionapplication.GenerateSourceDocumentCommand) (ingestionapplication.GenerateSourceDocumentResult, error)
}

type SourceDocumentGenerationHandler struct {
	generator sourceDocumentGenerator
}

func NewSourceDocumentGenerationHandler(generator *ingestionapplication.SourceDocumentGenerationService) (*SourceDocumentGenerationHandler, error) {
	if generator == nil {
		return nil, fmt.Errorf("source document generator is required")
	}
	return newSourceDocumentGenerationHandler(generator)
}

func newSourceDocumentGenerationHandler(generator sourceDocumentGenerator) (*SourceDocumentGenerationHandler, error) {
	if generator == nil {
		return nil, fmt.Errorf("source document generator is required")
	}
	return &SourceDocumentGenerationHandler{generator: generator}, nil
}

func (handler *SourceDocumentGenerationHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindGenerateSourceDocument); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.generator == nil {
		return queue.NewRetryableError(fmt.Errorf("source document generation handler is unavailable"))
	}
	args, err := decodeGenerateSourceDocumentJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	if args.TraceID != "" {
		ctx = sharedrequestcontext.WithTraceID(ctx, args.TraceID)
	}
	_, err = handler.generator.Generate(ctx, ingestionapplication.GenerateSourceDocumentCommand{
		EvidenceReferenceID: args.EvidenceReferenceID,
	})
	return queue.ClassifyHandlerError(ctx, err)
}

func decodeGenerateSourceDocumentJobArgs(encoded []byte) (GenerateSourceDocumentJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args GenerateSourceDocumentJobArgs
	if err := decoder.Decode(&args); err != nil {
		return GenerateSourceDocumentJobArgs{}, fmt.Errorf("decode source document generation job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return GenerateSourceDocumentJobArgs{}, fmt.Errorf("source document generation job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return GenerateSourceDocumentJobArgs{}, err
	}
	return args, nil
}

func validOptionalTraceID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 32 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}
