package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"go.uber.org/zap"
)

// XClient manages the X API v2 Filtered Stream connection.
type XClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewXClient creates an X API client.
func NewXClient(baseURL, token string) *XClient {
	return &XClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetRules replaces all Filtered Stream rules with the given ones.
func (c *XClient) SetRules(ctx context.Context, rules []dto.StreamRule) error {
	existing, err := c.getRules(ctx)
	if err != nil {
		return fmt.Errorf("get existing rules: %w", err)
	}
	if len(existing) > 0 {
		ids := make([]string, len(existing))
		for i, r := range existing {
			ids[i] = r.ID
		}
		delPayload, _ := json.Marshal(map[string]interface{}{
			"delete": map[string]interface{}{
				"ids": ids,
			},
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/2/tweets/search/stream/rules",
			strings.NewReader(string(delPayload)),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("delete rules: %w", err)
		}
		resp.Body.Close()
	}

	if len(rules) == 0 {
		return nil
	}
	addPayload, _ := json.Marshal(map[string]interface{}{
		"add": rules,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/2/tweets/search/stream/rules",
		strings.NewReader(string(addPayload)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("add rules: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add rules failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

// ConnectStream establishes a Filtered Stream connection.
func (c *XClient) ConnectStream(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/2/tweets/search/stream?expansions=author_id&tweet.fields=created_at&user.fields=name,username",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream connection failed: status=%d", resp.StatusCode)
	}
	return resp.Body, nil
}

// ParseTweet parses a single JSON line from the stream into a Tweet.
func ParseTweet(data []byte) (*dto.Tweet, error) {
	var raw struct {
		Data struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			AuthorID  string `json:"author_id"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
		Includes struct {
			Users []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Username string `json:"username"`
			} `json:"users"`
		} `json:"includes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal tweet: %w", err)
	}
	if raw.Data.ID == "" {
		return nil, fmt.Errorf("empty tweet data")
	}
	tweet := &dto.Tweet{
		ID:        raw.Data.ID,
		Text:      raw.Data.Text,
		AuthorID:  raw.Data.AuthorID,
		CreatedAt: raw.Data.CreatedAt,
	}
	for _, u := range raw.Includes.Users {
		if u.ID == raw.Data.AuthorID {
			tweet.AuthorName = u.Name
			tweet.AuthorHandle = u.Username
			break
		}
	}
	return tweet, nil
}

// ReadStream reads one tweet line from the scanner.
func ReadStream(scanner *bufio.Scanner) (*dto.Tweet, error) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		return ParseTweet([]byte(line))
	}
	return nil, scanner.Err()
}

func (c *XClient) getRules(ctx context.Context) ([]dto.StreamRule, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/2/tweets/search/stream/rules", nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data []dto.StreamRule `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil
	}
	return result.Data, nil
}

// CollectRepository defines the persistence interface needed by the collector.
type CollectRepository interface {
	ListActiveMonitors(ctx context.Context) ([]entity.KeywordMonitor, error)
	UpsertPost(ctx context.Context, p *entity.PlatformPost) error
	CreateHit(ctx context.Context, hit *entity.MonitorPostHit) error
}

// CollectService runs the X Filtered Stream collector.
type CollectService struct {
	client    *XClient
	embedder  *EmbeddingService
	repo      CollectRepository
	threshold float64
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
}

// NewCollectService creates a new collection service.
func NewCollectService(client *XClient, embedder *EmbeddingService, repo CollectRepository, threshold float64) *CollectService {
	if threshold <= 0 {
		threshold = 0.7
	}
	return &CollectService{
		client:    client,
		embedder:  embedder,
		repo:      repo,
		threshold: threshold,
	}
}

// Start begins the Filtered Stream connection and processes incoming tweets.
func (s *CollectService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	monitors, err := s.repo.ListActiveMonitors(ctx)
	if err != nil {
		return fmt.Errorf("list active monitors: %w", err)
	}
	rules := make([]dto.StreamRule, 0, len(monitors))
	for _, m := range monitors {
		rules = append(rules, dto.StreamRule{
			Value: m.QueryText,
			Tag:   fmt.Sprintf("monitor_%d", m.ID),
		})
	}
	if len(rules) > 0 {
		if err := s.client.SetRules(ctx, rules); err != nil {
			return fmt.Errorf("set stream rules: %w", err)
		}
		logging.L().Info("stream rules registered", zap.Int("count", len(rules)))
	} else {
		logging.L().Warn("no active monitors found, stream will receive no rules")
	}

	s.wg.Add(1)
	go s.runLoop(ctx)
	return nil
}

// Stop gracefully shuts down the collector.
func (s *CollectService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.running = false
}

func (s *CollectService) runLoop(ctx context.Context) {
	defer s.wg.Done()
	backoff := 1 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			logging.L().Info("collector stream loop stopped")
			return
		default:
		}

		body, err := s.client.ConnectStream(ctx)
		if err != nil {
			logging.L().Error("collector stream connect failed",
				zap.Error(err),
				zap.Duration("reconnect_in", backoff),
			)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = time.Duration(math.Min(
					float64(backoff*2),
					float64(maxBackoff),
				))
			}
			continue
		}
		backoff = 1 * time.Second

		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for {
			tweet, err := ReadStream(scanner)
			if err != nil {
				if err == io.EOF || err == context.Canceled {
					break
				}
				logging.L().Error("collector stream read error", zap.Error(err))
				break
			}
			if tweet != nil {
				s.processTweet(ctx, tweet)
			}
		}
		body.Close()
	}
}

func (s *CollectService) processTweet(ctx context.Context, tweet *dto.Tweet) {
	log := logging.L().With(zap.String("tweet_id", tweet.ID))

	emb, err := s.embedder.Embed(ctx, tweet.Text)
	if err != nil {
		log.Warn("embedding failed, storing without vector", zap.Error(err))
	}

	post := &entity.PlatformPost{
		Platform:       "x",
		PlatformPostID: tweet.ID,
		AuthorName:     tweet.AuthorName,
		AuthorHandle:   tweet.AuthorHandle,
		ContentText:    tweet.Text,
		PublishedAt:    parseTime(tweet.CreatedAt),
	}
	if err == nil {
		post.Embedding = &emb
	}
	if err := s.repo.UpsertPost(ctx, post); err != nil {
		log.Error("failed to upsert post", zap.Error(err))
		return
	}

	if post.Embedding == nil {
		return
	}

	monitors, err := s.repo.ListActiveMonitors(ctx)
	if err != nil {
		log.Error("failed to list monitors for matching", zap.Error(err))
		return
	}
	for _, m := range monitors {
		if m.QueryEmbedding == nil {
			continue
		}
		sim := cosineSimilarity(*post.Embedding, *m.QueryEmbedding)
		if sim >= s.threshold {
			now := time.Now()
			hit := &entity.MonitorPostHit{
				MonitorID:            m.ID,
				PostID:               post.ID,
				RelevanceScore:       sim,
				FreshnessScore:       1.0,
				AuthorInfluenceScore: 0.5,
				FinalScore:           sim*0.7 + 0.3,
				FirstSeenAt:          now,
				LastSeenAt:           now,
			}
			if err := s.repo.CreateHit(ctx, hit); err != nil {
				log.Error("failed to record hit",
					zap.Int64("monitor_id", m.ID),
					zap.Error(err),
				)
			}
		}
	}
}

func cosineSimilarity(a, b [384]float32) float64 {
	var dot, normA, normB float64
	for i := range 384 {
		va, vb := float64(a[i]), float64(b[i])
		dot += va * vb
		normA += va * va
		normB += vb * vb
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
