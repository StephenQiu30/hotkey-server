package application

import (
	"context"
	"fmt"
	"strings"
)

const (
	DocumentEmbeddingReasonPolicyUnavailable = "embedding_policy_unavailable"
	DocumentEmbeddingReasonModelUnavailable  = "embedding_model_unavailable"
)

type ProduceDocumentEmbeddingCommand struct {
	DocumentVersionID          int64
	SourceConnectionID         int64
	EmbedLocalRightsDecisionID int64
	RetainRightsDecisionID     int64
	NormalizedTextSHA256       string
	Plaintext                  string
}

type ProduceDocumentEmbeddingResult struct {
	DocumentVersionID int64
	Availability      SourceDocumentAvailability
	UnavailableReason string
	Receipt           *DocumentEmbeddingReceiptResult
}

type DocumentEmbeddingProducer interface {
	ProduceDocumentEmbedding(context.Context, ProduceDocumentEmbeddingCommand) (ProduceDocumentEmbeddingResult, error)
}

func validateProducedDocumentEmbedding(command ProduceDocumentEmbeddingCommand, result ProduceDocumentEmbeddingResult) error {
	if command.DocumentVersionID <= 0 || command.SourceConnectionID <= 0 || command.EmbedLocalRightsDecisionID <= 0 ||
		command.RetainRightsDecisionID <= 0 || !validLowerHexSHA256(command.NormalizedTextSHA256) ||
		strings.TrimSpace(command.Plaintext) == "" || result.DocumentVersionID != command.DocumentVersionID {
		return fmt.Errorf("document embedding identity is invalid")
	}
	switch result.Availability {
	case SourceDocumentUnavailable:
		if result.Receipt != nil || result.UnavailableReason != DocumentEmbeddingReasonModelUnavailable {
			return fmt.Errorf("unavailable document embedding result is invalid")
		}
	case SourceDocumentAvailable:
		receipt := result.Receipt
		if result.UnavailableReason != "" || receipt == nil || receipt.EmbeddingID <= 0 ||
			receipt.DocumentVersionID != command.DocumentVersionID || receipt.SourceConnectionID != command.SourceConnectionID ||
			receipt.EmbedLocalRightsDecisionID != command.EmbedLocalRightsDecisionID ||
			receipt.RetainRightsDecisionID != command.RetainRightsDecisionID || receipt.ModelProfileID <= 0 ||
			receipt.ModelProfileVersion <= 0 || receipt.ModelVersion == "" ||
			receipt.NormalizedTextSHA256 != command.NormalizedTextSHA256 || receipt.AIRunID <= 0 ||
			receipt.CreatedAt.IsZero() || !receipt.RetentionUntil.After(receipt.CreatedAt) ||
			receipt.LifecycleState != RecallAssetLifecycleActive {
			return fmt.Errorf("available document embedding receipt is invalid")
		}
	default:
		return fmt.Errorf("document embedding availability is invalid")
	}
	return nil
}
