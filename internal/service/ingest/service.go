package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

var ErrInvalidInput = errors.New("invalid input")

type Repository interface {
	FindByCanonicalURL(context.Context, string) (content.SourceItem, error)
	FindByContentHash(context.Context, string) (content.SourceItem, error)
	Create(context.Context, content.SourceItem) (content.SourceItem, error)
}

type Queue interface {
	Enqueue(context.Context, queue.EnqueueRequest) (queue.Job, error)
}

type Service struct {
	repo  Repository
	queue Queue
	now   func() time.Time
}

type Input struct {
	SourceID    string
	Title       string
	Snippet     string
	URL         string
	Language    string
	PublishedAt *time.Time
}

type Result struct {
	Item    content.SourceItem
	Created bool
}

func NewService(repo Repository, jobQueue Queue) *Service {
	return &Service{repo: repo, queue: jobQueue, now: time.Now}
}

func (s *Service) Ingest(ctx context.Context, input Input) (Result, error) {
	item, err := s.buildItem(input)
	if err != nil {
		return Result{}, err
	}
	if existing, err := s.repo.FindByCanonicalURL(ctx, item.CanonicalURL); err == nil {
		return Result{Item: existing, Created: false}, nil
	} else if !errors.Is(err, content.ErrNotFound) {
		return Result{}, err
	}

	if existing, err := s.repo.FindByContentHash(ctx, item.ContentHash); err == nil {
		item.Status = content.ItemStatusDuplicate
		item.DuplicateOfItemID = existing.ID
	} else if !errors.Is(err, content.ErrNotFound) {
		return Result{}, err
	}

	created, err := s.repo.Create(ctx, item)
	if err != nil {
		if errors.Is(err, content.ErrAlreadyExists) {
			if existing, findErr := s.repo.FindByCanonicalURL(ctx, item.CanonicalURL); findErr == nil {
				return Result{Item: existing, Created: false}, nil
			}
			if existing, findErr := s.repo.FindByContentHash(ctx, item.ContentHash); findErr == nil {
				return Result{Item: existing, Created: false}, nil
			}
		}
		return Result{}, err
	}
	if created.Status == content.ItemStatusPrimary {
		if err := s.enqueueEmbedding(ctx, created.ID); err != nil {
			return Result{}, err
		}
	}
	return Result{Item: created, Created: true}, nil
}

func (s *Service) buildItem(input Input) (content.SourceItem, error) {
	sourceID := strings.TrimSpace(input.SourceID)
	title := strings.TrimSpace(input.Title)
	snippet := strings.TrimSpace(input.Snippet)
	if sourceID == "" || title == "" || snippet == "" {
		return content.SourceItem{}, ErrInvalidInput
	}
	canonicalURL, err := content.CanonicalURL(input.URL)
	if err != nil {
		return content.SourceItem{}, ErrInvalidInput
	}
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = "unknown"
	}
	now := s.now().UTC()
	return content.SourceItem{
		ID:           content.NewID(),
		SourceID:     sourceID,
		Title:        title,
		Snippet:      snippet,
		RawURL:       strings.TrimSpace(input.URL),
		CanonicalURL: canonicalURL,
		PublishedAt:  cloneTime(input.PublishedAt),
		ContentHash: content.ContentHash(content.HashInput{
			Title:        title,
			Snippet:      snippet,
			CanonicalURL: canonicalURL,
		}),
		Language:  language,
		Status:    content.ItemStatusPrimary,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Service) enqueueEmbedding(ctx context.Context, itemID string) error {
	if s.queue == nil {
		return nil
	}
	payload, err := json.Marshal(queue.GenerateEmbeddingPayload{ItemID: itemID})
	if err != nil {
		return err
	}
	_, err = s.queue.Enqueue(ctx, queue.EnqueueRequest{
		Type:           queue.JobTypeGenerateEmbedding,
		Payload:        payload,
		IdempotencyKey: "generate_embedding:" + itemID,
	})
	return err
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
