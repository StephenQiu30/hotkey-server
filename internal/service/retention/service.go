package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
)

var ErrInvalidInput = errors.New("invalid input")

type Store interface {
	Delete(ctx context.Context, key string) error
	ListExpired(ctx context.Context, bucket string, before time.Time) ([]objectstorage.Object, error)
	ListByPrefix(ctx context.Context, prefix string) ([]objectstorage.Object, error)
}

type Service struct {
	store  Store
	logger *slog.Logger
	now    func() time.Time
}

func NewService(store Store, logger *slog.Logger) *Service {
	if store == nil {
		panic("retention service requires store")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		logger: logger,
		now:    time.Now,
	}
}

// CleanupExpired deletes all objects whose ExpiresAt is before now.
// Returns the number of objects deleted and any errors encountered.
func (s *Service) CleanupExpired(ctx context.Context, bucket string) (int, error) {
	now := s.now().UTC()
	expired, err := s.store.ListExpired(ctx, bucket, now)
	if err != nil {
		return 0, fmt.Errorf("list expired objects: %w", err)
	}

	deleted := 0
	var lastErr error
	for _, obj := range expired {
		if err := s.store.Delete(ctx, obj.Key); err != nil {
			s.logger.Warn("failed to delete expired object", "key", obj.Key, "error", err)
			lastErr = err
			continue
		}
		deleted++
		s.logger.Info("deleted expired object", "key", obj.Key, "expires_at", obj.Metadata.ExpiresAt)
	}

	if lastErr != nil && deleted == 0 {
		return 0, lastErr
	}
	return deleted, nil
}

// DeleteUserObjects deletes all objects belonging to a specific user.
// Returns the number of objects deleted and any errors encountered.
func (s *Service) DeleteUserObjects(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, ErrInvalidInput
	}

	// List all objects with the user prefix
	objects, err := s.store.ListByPrefix(ctx, userID+"/")
	if err != nil {
		return 0, fmt.Errorf("list user objects: %w", err)
	}

	deleted := 0
	var lastErr error
	for _, obj := range objects {
		if obj.Metadata.UserID != userID {
			continue
		}
		if err := s.store.Delete(ctx, obj.Key); err != nil {
			s.logger.Warn("failed to delete user object", "key", obj.Key, "user_id", userID, "error", err)
			lastErr = err
			continue
		}
		deleted++
		s.logger.Info("deleted user object", "key", obj.Key, "user_id", userID)
	}

	if lastErr != nil && deleted == 0 {
		return 0, lastErr
	}
	return deleted, nil
}
