package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/StephenQiu30/hotkey-server/internal/platform/x"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

// XConnectorAdapter wraps x.Client to implement PlatformConnector.
type XConnectorAdapter struct {
	client *x.Client
	token  string
}

// NewXConnectorAdapter creates a new adapter wrapping an x.Client.
func NewXConnectorAdapter(client *x.Client, token string) *XConnectorAdapter {
	return &XConnectorAdapter{client: client, token: token}
}

// SearchPosts fetches posts from the X search API and normalizes them.
func (a *XConnectorAdapter) SearchPosts(ctx context.Context, query string, cursor string) ([]PostResult, string, error) {
	searchURL := fmt.Sprintf("https://api.x.com/2/tweets/search/recent?query=%s", url.QueryEscape(query))
	if cursor != "" {
		searchURL += "&next_token=" + url.QueryEscape(cursor)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("x api returned %d: %s", resp.StatusCode, string(body))
	}

	posts, meta, err := a.client.ParseSearchResponse(body)
	if err != nil {
		return nil, "", err
	}

	results := make([]PostResult, 0, len(posts))
	for _, p := range posts {
		results = append(results, PostResult{
			ID:           p.ID,
			AuthorID:     p.AuthorID,
			AuthorName:   p.AuthorName,
			AuthorHandle: p.AuthorHandle,
			Text:         p.Text,
			Language:     p.Language,
			PublishedAt:  p.PublishedAt,
			LikeCount:    p.LikeCount,
			ReplyCount:   p.ReplyCount,
			RepostCount:  p.RepostCount,
			QuoteCount:   p.QuoteCount,
			ViewCount:    p.ViewCount,
		})
	}

	return results, meta.NextCursor, nil
}

// ScorerAdapter wraps scoring.Service to implement HitScorer.
type ScorerAdapter struct {
	svc *scoring.Service
}

// NewScorerAdapter creates a new adapter wrapping a scoring.Service.
func NewScorerAdapter(svc *scoring.Service) *ScorerAdapter {
	return &ScorerAdapter{svc: svc}
}

// ScoreHit computes scores for a hit using the scoring service.
func (a *ScorerAdapter) ScoreHit(hitID int64, post PostResult, matchedKeywords []string, totalKeywords int, publishedMinutesAgo float64) error {
	return a.svc.ScoreHit(scoring.ScoreHitInput{
		HitID:               hitID,
		LikeCount:           post.LikeCount,
		ReplyCount:          post.ReplyCount,
		RepostCount:         post.RepostCount,
		QuoteCount:          post.QuoteCount,
		ViewCount:           post.ViewCount,
		MatchedKeywords:     matchedKeywords,
		TotalKeywords:       totalKeywords,
		PublishedMinutesAgo: publishedMinutesAgo,
	})
}

// DBRunRepository implements RunRepository using PostgreSQL.
type DBRunRepository struct {
	db *sql.DB
}

// NewDBRunRepository creates a new database-backed run repository.
func NewDBRunRepository(db *sql.DB) *DBRunRepository {
	return &DBRunRepository{db: db}
}

// CreateRun inserts a new monitor run record.
func (r *DBRunRepository) CreateRun(ctx context.Context, run MonitorRun) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO monitor_runs (monitor_id, platform, run_type, status, started_at, fetched_count, stored_count, error_message)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		run.MonitorID, run.Platform, run.RunType, run.Status, run.StartedAt,
		run.FetchedCount, run.StoredCount, run.ErrorMessage,
	).Scan(&id)
	return id, err
}

// UpdateRun updates an existing monitor run record.
func (r *DBRunRepository) UpdateRun(ctx context.Context, runID int64, run MonitorRun) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE monitor_runs SET status = $1, finished_at = $2, fetched_count = $3,
		 stored_count = $4, error_message = $5 WHERE id = $6`,
		run.Status, run.FinishedAt, run.FetchedCount, run.StoredCount, run.ErrorMessage, runID,
	)
	return err
}

// DBPostRepository implements PostRepository using PostgreSQL.
type DBPostRepository struct {
	db *sql.DB
}

// NewDBPostRepository creates a new database-backed post repository.
func NewDBPostRepository(db *sql.DB) *DBPostRepository {
	return &DBPostRepository{db: db}
}

// UpsertPost inserts or updates a platform post and returns its ID.
func (r *DBPostRepository) UpsertPost(ctx context.Context, post PostResult) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO platform_posts
			(platform, platform_post_id, author_platform_id, author_name, author_handle,
			 content_text, content_lang, published_at,
			 like_count, reply_count, repost_count, quote_count, view_count)
		 VALUES ('x', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (platform, platform_post_id) DO UPDATE SET
			 author_name = EXCLUDED.author_name,
			 author_handle = EXCLUDED.author_handle,
			 content_text = EXCLUDED.content_text,
			 like_count = EXCLUDED.like_count,
			 reply_count = EXCLUDED.reply_count,
			 repost_count = EXCLUDED.repost_count,
			 quote_count = EXCLUDED.quote_count,
			 view_count = EXCLUDED.view_count,
			 updated_at = now()
		 RETURNING id`,
		post.ID, post.AuthorID, post.AuthorName, post.AuthorHandle,
		post.Text, post.Language, post.PublishedAt,
		post.LikeCount, post.ReplyCount, post.RepostCount, post.QuoteCount, post.ViewCount,
	).Scan(&id)
	return id, err
}

// DBHitRepository implements HitRepository using PostgreSQL.
type DBHitRepository struct {
	db *sql.DB
}

// NewDBHitRepository creates a new database-backed hit repository.
func NewDBHitRepository(db *sql.DB) *DBHitRepository {
	return &DBHitRepository{db: db}
}

// UpsertHit inserts or updates a monitor-post hit.
func (r *DBHitRepository) UpsertHit(ctx context.Context, hit HitResult) error {
	kwJSON := "["
	for i, kw := range hit.MatchedKeywords {
		if i > 0 {
			kwJSON += ","
		}
		kwJSON += `"` + kw + `"`
	}
	kwJSON += "]"

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO monitor_post_hits (monitor_id, post_id, matched_keywords)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (monitor_id, post_id) DO UPDATE SET
			 matched_keywords = EXCLUDED.matched_keywords,
			 last_seen_at = now()`,
		hit.MonitorID, hit.PostID, kwJSON,
	)
	return err
}

// DBHitScorerRepo implements scoring.HitRepository using PostgreSQL.
type DBHitScorerRepo struct {
	db *sql.DB
}

// NewDBHitScorerRepo creates a new database-backed hit scorer repository.
func NewDBHitScorerRepo(db *sql.DB) *DBHitScorerRepo {
	return &DBHitScorerRepo{db: db}
}

// UpdateScores updates the scoring columns on a monitor_post_hits row.
func (r *DBHitScorerRepo) UpdateScores(hitID int64, score scoring.SavedScore) error {
	_, err := r.db.Exec(
		`UPDATE monitor_post_hits SET
			heat_score = $1, relevance_score = $2, freshness_score = $3,
			author_influence_score = $4, final_score = $5
		 WHERE id = $6`,
		score.HeatScore, score.RelevanceScore, score.FreshnessScore,
		score.AuthorInfluenceScore, score.FinalScore, hitID,
	)
	return err
}

// DBPostCandidateProvider fetches unclustered posts from the database.
type DBPostCandidateProvider struct {
	db *sql.DB
}

// NewDBPostCandidateProvider creates a new database-backed post candidate provider.
func NewDBPostCandidateProvider(db *sql.DB) *DBPostCandidateProvider {
	return &DBPostCandidateProvider{db: db}
}

// GetUnclusteredPosts returns posts not yet assigned to any topic.
func (p *DBPostCandidateProvider) GetUnclusteredPosts(monitorID int64) ([]PostCandidate, error) {
	rows, err := p.db.Query(
		`SELECT pp.id, pp.content_text
		 FROM monitor_post_hits mph
		 JOIN platform_posts pp ON pp.id = mph.post_id
		 WHERE mph.monitor_id = $1
		   AND NOT EXISTS (
		     SELECT 1 FROM topic_posts tp WHERE tp.post_id = pp.id
		   )
		 ORDER BY mph.final_score DESC
		 LIMIT 100`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []PostCandidate
	for rows.Next() {
		var c PostCandidate
		if err := rows.Scan(&c.PostID, &c.Text); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// TopicPersisterAdapter wraps topic.Repository to implement TopicPersister.
type TopicPersisterAdapter struct {
	repo topic.Repository
}

// NewTopicPersisterAdapter creates a new adapter wrapping a topic.Repository.
func NewTopicPersisterAdapter(repo topic.Repository) *TopicPersisterAdapter {
	return &TopicPersisterAdapter{repo: repo}
}

// UpsertTopic delegates to the topic repository.
func (a *TopicPersisterAdapter) UpsertTopic(monitorID int64, t topic.Topic) (int64, error) {
	return a.repo.UpsertTopic(monitorID, t)
}

// AddPostToTopic delegates to the topic repository.
func (a *TopicPersisterAdapter) AddPostToTopic(topicID, postID int64, membershipScore float64) error {
	return a.repo.AddPostToTopic(topicID, postID, membershipScore)
}

// DBTopicProvider fetches topic data from the database for snapshot building.
type DBTopicProvider struct {
	db *sql.DB
}

// NewDBTopicProvider creates a new database-backed topic provider.
func NewDBTopicProvider(db *sql.DB) *DBTopicProvider {
	return &DBTopicProvider{db: db}
}

// GetTopicDataForMonitor returns aggregated topic data for snapshot building.
func (p *DBTopicProvider) GetTopicDataForMonitor(monitorID int64) ([]TopicData, error) {
	rows, err := p.db.Query(
		`SELECT t.id,
		        COUNT(tp.id) AS post_count,
		        0 AS unique_author_count,
		        0 AS engagement_sum,
		        COALESCE(t.current_heat_score, 0) AS heat_score,
		        COALESCE(ts_prev.heat_score, 0) AS previous_heat
		 FROM topics t
		 LEFT JOIN topic_posts tp ON tp.topic_id = t.id
		 LEFT JOIN LATERAL (
		     SELECT heat_score FROM topic_snapshots
		     WHERE topic_id = t.id ORDER BY snapshot_time DESC LIMIT 1
		 ) ts_prev ON true
		 WHERE t.monitor_id = $1 AND t.status = 'active'
		 GROUP BY t.id, t.current_heat_score, ts_prev.heat_score`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []TopicData
	for rows.Next() {
		var d TopicData
		if err := rows.Scan(&d.TopicID, &d.PostCount, &d.UniqueAuthorCount, &d.EngagementSum, &d.HeatScore, &d.PreviousHeat); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

// DBDeliveryRepository implements DeliveryRepository using PostgreSQL.
type DBDeliveryRepository struct {
	db *sql.DB
}

// NewDBDeliveryRepository creates a new database-backed delivery repository.
func NewDBDeliveryRepository(db *sql.DB) *DBDeliveryRepository {
	return &DBDeliveryRepository{db: db}
}

// CreateDelivery inserts a new delivery record.
func (r *DBDeliveryRepository) CreateDelivery(ctx context.Context, d EmailDelivery) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO email_deliveries (notification_id, recipient_email, provider, status)
		 VALUES ($1, $2, $3, $4)`,
		d.NotificationID, d.RecipientEmail, d.Provider, d.Status,
	)
	return err
}

// UpdateDeliveryStatus updates the status of a delivery record.
func (r *DBDeliveryRepository) UpdateDeliveryStatus(ctx context.Context, notificationID int64, status string, providerMsgID string, errMsg string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE email_deliveries SET status = $1, provider_message_id = $2, error_message = $3
		 WHERE notification_id = $4`,
		status, providerMsgID, errMsg, notificationID,
	)
	return err
}

// GetPendingDeliveries returns pending delivery records up to limit.
func (r *DBDeliveryRepository) GetPendingDeliveries(ctx context.Context, limit int) ([]EmailDelivery, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, notification_id, recipient_email, provider, provider_message_id, status, error_message, sent_at
		 FROM email_deliveries WHERE status = 'pending' ORDER BY id ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []EmailDelivery
	for rows.Next() {
		var d EmailDelivery
		if err := rows.Scan(&d.ID, &d.NotificationID, &d.RecipientEmail, &d.Provider, &d.ProviderMessageID, &d.Status, &d.ErrorMessage, &d.SentAt); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// DBUserEmailLookup resolves user email from notification ID using the database.
type DBUserEmailLookup struct {
	db *sql.DB
}

// NewDBUserEmailLookup creates a new database-backed email lookup.
func NewDBUserEmailLookup(db *sql.DB) *DBUserEmailLookup {
	return &DBUserEmailLookup{db: db}
}

// ResolveEmail resolves a user's email address from a notification ID.
func (l *DBUserEmailLookup) ResolveEmail(ctx context.Context, notificationID int64) (string, error) {
	var email string
	err := l.db.QueryRowContext(ctx,
		`SELECT u.email FROM user_notifications un
		 JOIN users u ON u.id = un.user_id
		 WHERE un.id = $1`, notificationID,
	).Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}

// MonitorLister provides a list of active monitors for the worker to iterate.
type MonitorLister interface {
	ListActiveIDs(ctx context.Context) ([]int64, error)
}

// DBMonitorLister implements MonitorLister using PostgreSQL.
type DBMonitorLister struct {
	db *sql.DB
}

// NewDBMonitorLister creates a new database-backed monitor lister.
func NewDBMonitorLister(db *sql.DB) *DBMonitorLister {
	return &DBMonitorLister{db: db}
}

// ListActiveIDs returns IDs of all active monitors.
func (l *DBMonitorLister) ListActiveIDs(ctx context.Context) ([]int64, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT id FROM keyword_monitors WHERE status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
