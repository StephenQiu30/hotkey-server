package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
)

// EmbeddingSpace pins every read to one profile revision and one model
// version. It is intentionally a public read contract: downstream modules
// need no visibility into ai_runs or embedding tables.
type EmbeddingSpace struct {
	ModelProfileID, ModelProfileVersion int64
	ModelVersion                        string
}

func (space EmbeddingSpace) valid() bool {
	return space.ModelProfileID > 0 && space.ModelProfileVersion > 0 && strings.TrimSpace(space.ModelVersion) != ""
}

// ActiveEmbedding is a safe vector plus the immutable space that produced it.
type ActiveEmbedding struct {
	EmbeddingSpace
	Vector []float32
}

// EmbeddingNeighbor is a target ID with cosine distance in the same exact
// space as the active query vector.
type EmbeddingNeighbor struct {
	TargetID int64
	Distance float64
}

type embeddingQueryRepository interface {
	ActiveEmbedding(context.Context, intelligencepostgres.EmbeddingTarget, int64, int64, int64, string) ([]float32, bool, error)
	NearestEmbeddings(context.Context, intelligencepostgres.EmbeddingTarget, int64, int64, string, []float32, int) ([]intelligencepostgres.EmbeddingMatch, error)
	NearestPublishedMonitorEmbeddings(context.Context, int64, int64, string, []float32, int) ([]intelligencepostgres.EmbeddingMatch, error)
}

// EmbeddingQueryService is intelligence's sole public read-only embedding
// facade. It deliberately exports only Content lookup and Monitor-neighbor
// lookup needed by relevance matching; it has no write or run-control API.
type EmbeddingQueryService struct{ repository embeddingQueryRepository }

func NewEmbeddingQueryService(repository *intelligencepostgres.Repository) (*EmbeddingQueryService, error) {
	if repository == nil {
		return nil, fmt.Errorf("AI embedding query repository is required")
	}
	return &EmbeddingQueryService{repository: repository}, nil
}

func (service *EmbeddingQueryService) ActiveContent(ctx context.Context, contentID int64, space EmbeddingSpace) (ActiveEmbedding, bool, error) {
	if service == nil || service.repository == nil || contentID <= 0 || !space.valid() {
		return ActiveEmbedding{}, false, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	vector, found, err := service.repository.ActiveEmbedding(ctx, intelligencepostgres.EmbeddingTargetContent, contentID, space.ModelProfileID, space.ModelProfileVersion, space.ModelVersion)
	if err != nil || !found {
		return ActiveEmbedding{}, found, err
	}
	return ActiveEmbedding{EmbeddingSpace: space, Vector: vector}, true, nil
}

func (service *EmbeddingQueryService) NearestMonitors(ctx context.Context, embedding ActiveEmbedding, limit int) ([]EmbeddingNeighbor, error) {
	if service == nil || service.repository == nil || !embedding.EmbeddingSpace.valid() || limit < 1 || limit > 12 {
		return nil, domain.NewError(domain.CodeAIModelProfileInvalid)
	}
	if err := domain.ValidateEmbedding(embedding.Vector); err != nil {
		return nil, err
	}
	matches, err := service.repository.NearestPublishedMonitorEmbeddings(ctx, embedding.ModelProfileID, embedding.ModelProfileVersion, embedding.ModelVersion, embedding.Vector, limit)
	if err != nil {
		return nil, err
	}
	neighbors := make([]EmbeddingNeighbor, 0, len(matches))
	for _, match := range matches {
		if match.TargetID <= 0 || match.ModelProfileVersion != embedding.ModelProfileVersion || match.ModelVersion != embedding.ModelVersion {
			return nil, domain.NewError(domain.CodeAIModelProfileInvalid)
		}
		neighbors = append(neighbors, EmbeddingNeighbor{TargetID: match.TargetID, Distance: match.Distance})
	}
	return neighbors, nil
}
