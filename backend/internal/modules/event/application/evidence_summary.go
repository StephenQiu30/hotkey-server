package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
)

var ErrInvalidEvidenceSummaryContract = errors.New("evidence summary contract is invalid")

type EvidenceSummarySentenceInputDTO struct {
	Text                    string
	EditorialNote           bool
	ClaimEvidenceVersionIDs []int64
	DecisionOrigin          string
	ModelRunID              *int64
	ActorUserID             *int64
}

type PublishEvidenceSummaryCommand struct {
	MicroEventID, ExpectedEventVersion int64
	SummaryProfileVersion              string
	Sentences                          []EvidenceSummarySentenceInputDTO
	IdempotencyKey                     string
	CreatedAt                          time.Time
}

type CommitEvidenceSummaryCommand struct {
	MicroEventID, EventVersion int64
	SummaryProfileVersion      string
	Sentences                  []EvidenceSummarySentenceInputDTO
	IdempotencyKey             string
	CommandFingerprint         string
	CreatedAt                  time.Time
}

type EvidenceSummarySentenceDTO struct {
	ID, Version, SummaryID  int64
	Ordinal                 int
	Text                    string
	EditorialNote           bool
	ClaimEvidenceVersionIDs []int64
	DecisionOrigin          string
	ModelRunID, ActorUserID *int64
	CreatedAt               time.Time
}

type EvidenceSummaryDTO struct {
	ID, Version, MicroEventID, EventVersion int64
	SummaryProfileVersion                   string
	Sentences                               []EvidenceSummarySentenceDTO
	CreatedAt                               time.Time
	Created                                 bool
}

type PublishEvidenceSummaryResult struct{ Summary EvidenceSummaryDTO }

type EvidenceSummaryRepository interface {
	CommitEvidenceSummary(context.Context, CommitEvidenceSummaryCommand) (EvidenceSummaryDTO, error)
}

type EvidenceSummaryService struct{ repository EvidenceSummaryRepository }

func NewEvidenceSummaryService(repository EvidenceSummaryRepository) (*EvidenceSummaryService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidEvidenceSummaryContract)
	}
	return &EvidenceSummaryService{repository: repository}, nil
}

func (service *EvidenceSummaryService) Publish(ctx context.Context, command PublishEvidenceSummaryCommand) (PublishEvidenceSummaryResult, error) {
	command.SummaryProfileVersion, command.IdempotencyKey = strings.TrimSpace(command.SummaryProfileVersion), strings.TrimSpace(command.IdempotencyKey)
	command.CreatedAt = command.CreatedAt.UTC()
	domainSentences := make([]eventdomain.EvidenceSummarySentence, len(command.Sentences))
	canonical := make([]EvidenceSummarySentenceInputDTO, len(command.Sentences))
	for index, sentence := range command.Sentences {
		canonical[index] = sentence
		canonical[index].Text = canonicalClaimPart(sentence.Text)
		canonical[index].DecisionOrigin = strings.TrimSpace(sentence.DecisionOrigin)
		canonical[index].ClaimEvidenceVersionIDs = append([]int64(nil), sentence.ClaimEvidenceVersionIDs...)
		domainSentences[index] = eventdomain.EvidenceSummarySentence{Text: canonical[index].Text, EditorialNote: sentence.EditorialNote,
			EvidenceIDs: canonical[index].ClaimEvidenceVersionIDs, DecisionOrigin: canonical[index].DecisionOrigin,
			ModelRunID: sentence.ModelRunID, ActorUserID: sentence.ActorUserID}
	}
	if service == nil || service.repository == nil || command.MicroEventID <= 0 || command.ExpectedEventVersion <= 0 ||
		command.SummaryProfileVersion == "" || len(command.SummaryProfileVersion) > 64 || command.IdempotencyKey == "" ||
		len(command.IdempotencyKey) > 96 || command.CreatedAt.IsZero() || eventdomain.ValidateEvidenceSummarySentences(domainSentences) != nil {
		return PublishEvidenceSummaryResult{}, ErrInvalidEvidenceSummaryContract
	}
	payload, _ := json.Marshal(struct {
		EventID, EventVersion int64
		Profile               string
		Sentences             []EvidenceSummarySentenceInputDTO
	}{command.MicroEventID, command.ExpectedEventVersion, command.SummaryProfileVersion, canonical})
	digest := sha256.Sum256(payload)
	summary, err := service.repository.CommitEvidenceSummary(ctx, CommitEvidenceSummaryCommand{MicroEventID: command.MicroEventID,
		EventVersion: command.ExpectedEventVersion, SummaryProfileVersion: command.SummaryProfileVersion,
		Sentences: canonical, IdempotencyKey: command.IdempotencyKey, CommandFingerprint: hex.EncodeToString(digest[:]),
		CreatedAt: command.CreatedAt})
	if err != nil {
		return PublishEvidenceSummaryResult{}, fmt.Errorf("commit evidence summary: %w", err)
	}
	if summary.ID <= 0 || summary.MicroEventID != command.MicroEventID || summary.EventVersion != command.ExpectedEventVersion ||
		summary.SummaryProfileVersion != command.SummaryProfileVersion || len(summary.Sentences) != len(canonical) {
		return PublishEvidenceSummaryResult{}, ErrInvalidEvidenceSummaryContract
	}
	for index := range canonical {
		if summary.Sentences[index].Ordinal != index || summary.Sentences[index].Text != canonical[index].Text ||
			summary.Sentences[index].EditorialNote != canonical[index].EditorialNote ||
			summary.Sentences[index].DecisionOrigin != canonical[index].DecisionOrigin ||
			!equalEvidenceSummaryIDs(summary.Sentences[index].ClaimEvidenceVersionIDs, canonical[index].ClaimEvidenceVersionIDs) ||
			!equalEvidenceSummaryOptionalID(summary.Sentences[index].ModelRunID, canonical[index].ModelRunID) ||
			!equalEvidenceSummaryOptionalID(summary.Sentences[index].ActorUserID, canonical[index].ActorUserID) {
			return PublishEvidenceSummaryResult{}, ErrInvalidEvidenceSummaryContract
		}
	}
	return PublishEvidenceSummaryResult{Summary: summary}, nil
}

func equalEvidenceSummaryIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalEvidenceSummaryOptionalID(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
