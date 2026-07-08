// Package collect provides X (Twitter) Filtered Stream collection and
// cosine-similarity matching against active keyword monitors.
package collect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/embedding"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"go.uber.org/zap"
)

// Service runs the X Filtered Stream collector.
type Service struct {
	client     *XClient
	embedder   *embedding.Service
	repo       *repository.CollectRepo
	threshold  float64
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    bool
}

// NewService creates a new collection service.
func NewService(client *XClient, embedder *embedding.Service, repo *repository.CollectRepo, threshold float64) *Service {
	if threshold <= 0 {
		threshold = 0.7
	}
	return &Service{
		client:    client,
		embedder:  embedder,
		repo:      repo,
		threshold: threshold,
	}
}

// Start begins the Filtered Stream connection and processes incoming tweets.
// Returns after initial rule registration; the stream runs in a background goroutine.
func (s *Service) Start(ctx context.Context) error {
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
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.running = false
}

func (s *Service) runLoop(ctx context.Context) {
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

func (s *Service) processTweet(ctx context.Context, tweet *dto.Tweet) {
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

