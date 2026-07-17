package postgres

import (
	"context"
	"fmt"
	"time"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// ReadFeed only joins published reports to an enabled RSS subscription whose
// token hash matches. Plain tokens never reach this adapter.
func (repository *Repository) ReadFeed(ctx context.Context, tokenHash string) (deliveryapplication.Feed, error) {
	if repository == nil || repository.runtime == nil {
		return deliveryapplication.Feed{}, sharedrepository.ErrUnavailable
	}
	if len(tokenHash) != 64 {
		return deliveryapplication.Feed{}, fmt.Errorf("%w: invalid feed token", sharedrepository.ErrInvalidInput)
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT r.title, r.period_end, ri.event_id, ri.title_snapshot, ri.summary_snapshot
FROM report_subscriptions s
JOIN reports r ON r.report_type = s.report_type AND r.monitor_id IS NOT DISTINCT FROM s.monitor_id AND r.status = 'published'
JOIN report_items ri ON ri.report_id = r.id
WHERE s.rss_token_hash = $1 AND s.channel = 'rss' AND s.enabled = true
  AND r.period_end = (SELECT max(r2.period_end) FROM reports r2 WHERE r2.report_type = s.report_type AND r2.monitor_id IS NOT DISTINCT FROM s.monitor_id AND r2.status = 'published')
ORDER BY ri.rank, ri.event_id`, tokenHash)
	if err != nil {
		return deliveryapplication.Feed{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	feed := deliveryapplication.Feed{Link: "https://hotkey.local/feeds", UpdatedAt: time.Time{}}
	for rows.Next() {
		var reportTitle, itemTitle, summary string
		var eventID int64
		var publishedAt time.Time
		if err := rows.Scan(&reportTitle, &publishedAt, &eventID, &itemTitle, &summary); err != nil {
			return deliveryapplication.Feed{}, sharedrepository.MapError(err)
		}
		if feed.Title == "" {
			feed.Title = reportTitle
		}
		if publishedAt.After(feed.UpdatedAt) {
			feed.UpdatedAt = publishedAt
		}
		feed.Items = append(feed.Items, deliveryapplication.FeedItem{ID: fmt.Sprintf("event-%d", eventID), Title: itemTitle, URL: fmt.Sprintf("https://hotkey.local/api/v1/events/%d", eventID), Summary: summary, PublishedAt: publishedAt})
	}
	if err := rows.Err(); err != nil {
		return deliveryapplication.Feed{}, sharedrepository.MapError(err)
	}
	if feed.Title == "" {
		return deliveryapplication.Feed{}, sharedrepository.ErrNotFound
	}
	return feed, nil
}

var _ interface {
	ReadFeed(context.Context, string) (deliveryapplication.Feed, error)
} = (*Repository)(nil)
