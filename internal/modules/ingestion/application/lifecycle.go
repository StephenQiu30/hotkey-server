package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

// DeleteBySourceItemResult reports only Content and evidence asset facts
// owned by ingestion. Content is tombstoned even if evidence deletion must be
// retried later.
type DeleteBySourceItemResult struct {
	Content             ingestiondomain.Content
	ContentChanged      bool
	AssetsDeleted       int
	AssetsDeletePending int
}

// DeleteBySourceItem tombstones one source Content fact before operating on
// its deterministic evidence assets. A failed object delete is durable as
// delete_pending so a repeated command can safely complete the transition.
func (service *Service) DeleteBySourceItem(ctx context.Context, sourceConnectionID int64, externalID string) (DeleteBySourceItemResult, error) {
	if !service.lifecycleReady() {
		return DeleteBySourceItemResult{}, errors.New("ingestion lifecycle service is not initialized")
	}
	if sourceConnectionID <= 0 || strings.TrimSpace(externalID) == "" {
		return DeleteBySourceItemResult{}, errors.New("source connection id and external id are required")
	}

	content, changed, err := service.contents.MarkDeleted(ctx, sourceConnectionID, externalID)
	if err != nil {
		return DeleteBySourceItemResult{}, fmt.Errorf("mark content deleted: %w", err)
	}
	result := DeleteBySourceItemResult{Content: content, ContentChanged: changed}
	if content.ID == 0 {
		return result, nil
	}

	deleteFailures := make([]error, 0)
	err = service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if err := lockSourceEvidenceTransaction(transactionCtx, transaction, sourceConnectionID); err != nil {
			return err
		}
		assets, err := service.contents.ListEvidenceAssets(transactionCtx, sourceConnectionID, content.ID)
		if err != nil {
			return fmt.Errorf("list content evidence assets: %w", err)
		}
		for _, asset := range assets {
			if err := service.evidence.Delete(transactionCtx, asset.ObjectKey); err != nil {
				if statusErr := service.contents.MarkAssetStatus(transactionCtx, asset.ObjectKey, ingestiondomain.AssetStatusDeletePending); statusErr != nil {
					return fmt.Errorf("record pending deletion for evidence %q: %w", asset.ObjectKey, statusErr)
				}
				result.AssetsDeletePending++
				deleteFailures = append(deleteFailures, fmt.Errorf("delete evidence %q: %w", asset.ObjectKey, err))
				continue
			}
			if err := service.contents.MarkAssetStatus(transactionCtx, asset.ObjectKey, ingestiondomain.AssetStatusDeleted); err != nil {
				return fmt.Errorf("mark deleted evidence %q: %w", asset.ObjectKey, err)
			}
			result.AssetsDeleted++
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if len(deleteFailures) != 0 {
		return result, errors.Join(deleteFailures...)
	}
	return result, nil
}

// ExpireBefore updates only active ingestion Content facts. Its repository
// contract excludes expired Content from the active cursor immediately and it
// neither deletes assets nor initiates downstream recalculation.
func (service *Service) ExpireBefore(ctx context.Context, before time.Time) (int, error) {
	if !service.lifecycleReady() {
		return 0, errors.New("ingestion lifecycle service is not initialized")
	}
	if before.IsZero() {
		return 0, errors.New("expiry time is required")
	}
	expired, err := service.contents.ExpireBefore(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("expire active content: %w", err)
	}
	return expired, nil
}

// ReconcileObjects removes unreferenced deterministic evidence objects for a
// source while preserving every non-deleted ingestion asset reference. The
// transaction lock matches ingestion writes and compensation so an object is
// never deleted while this application is creating its durable reference.
func (service *Service) ReconcileObjects(ctx context.Context, sourceConnectionID int64) (int, error) {
	if !service.lifecycleReady() || sourceConnectionID <= 0 {
		return 0, errors.New("content repository, evidence store, and source connection id are required")
	}
	deleted := 0
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if err := lockSourceEvidenceTransaction(transactionCtx, transaction, sourceConnectionID); err != nil {
			return err
		}
		prefix := fmt.Sprintf("evidence/v1/%d/", sourceConnectionID)
		keys, err := service.contents.ListAssetObjectKeys(transactionCtx, sourceConnectionID)
		if err != nil {
			return fmt.Errorf("list durable evidence assets: %w", err)
		}
		known := make(map[string]struct{}, len(keys))
		for _, key := range keys {
			known[key] = struct{}{}
		}
		receipts, err := service.evidence.ListPrefix(transactionCtx, prefix)
		if err != nil {
			return fmt.Errorf("list evidence objects: %w", err)
		}
		for _, receipt := range receipts {
			if _, found := known[receipt.ObjectKey]; found {
				continue
			}
			if err := service.evidence.Delete(transactionCtx, receipt.ObjectKey); err != nil {
				return fmt.Errorf("delete orphan evidence %q: %w", receipt.ObjectKey, err)
			}
			deleted++
		}
		return nil
	})
	if err != nil {
		return deleted, err
	}
	return deleted, nil
}

func (service *Service) lifecycleReady() bool {
	return service != nil && service.runtime != nil && service.contents != nil && service.evidence != nil
}
