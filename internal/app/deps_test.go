package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/redis"
)

// testDialer 实现 DepsDialer 接口，用于在测试中控制 DB/Redis ping 行为。
type testDialer struct {
	dbErr    error
	redisErr error
}

func (d *testDialer) PingDB(context.Context) error    { return d.dbErr }
func (d *testDialer) PingRedis(context.Context) error { return d.redisErr }

func TestNewDepsFailsWhenDBUnreachable(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://invalid:invalid@localhost:1/nonexistent",
		RedisURL:    "redis://127.0.0.1:6379/0",
	}
	_, err := NewDeps(cfg, WithDialer(&testDialer{
		dbErr: errors.New("connection refused"),
	}))
	if err == nil {
		t.Fatal("expected error when DB ping fails")
	}
	if !errors.Is(err, errDBRequired) {
		t.Fatalf("expected errDBRequired, got %v", err)
	}
}

func TestNewDepsFailsWhenRedisUnreachable(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey",
		RedisURL:    "redis://127.0.0.1:6379/0",
	}
	_, err := NewDeps(cfg, WithDialer(&testDialer{
		redisErr: errors.New("connection refused"),
	}))
	if err == nil {
		t.Fatal("expected error when Redis ping fails")
	}
	if !errors.Is(err, errRedisRequired) {
		t.Fatalf("expected errRedisRequired, got %v", err)
	}
}

func TestNewDepsSucceedsWithValidDeps(t *testing.T) {
	cfg := config.Config{
		DatabaseURL:                "postgres://hotkey:hotkey@localhost:5432/hotkey",
		RedisURL:                   "redis://127.0.0.1:6379/0",
		DashScopeAPIKey:            "test-key",
		CollectSourceID:            "test-source",
		HotspotSimilarityThreshold: 0.85,
		EmbeddingModel:             "text-embedding-v2",
	}

	// 创建一个不会 ping 真实服务的 mock dialer
	mockDB, _ := sql.Open("postgres", "postgres://invalid/nil")
	mockRedis := redis.NewClient("redis://127.0.0.1:1", redis.Options{})

	deps, err := NewDeps(cfg,
		WithDialer(&testDialer{}),
		WithDB(mockDB),
		WithRedisClient(mockRedis),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deps == nil {
		t.Fatal("expected non-nil deps")
	}
	defer deps.Close()

	// 验证核心依赖已初始化
	if deps.Cfg.DatabaseURL == "" {
		t.Error("expected Cfg to be set")
	}
	if deps.DB == nil {
		t.Error("expected DB to be set")
	}
	if deps.RedisClient == nil {
		t.Error("expected RedisClient to be set")
	}
	if deps.JobQueue == nil {
		t.Error("expected JobQueue to be set")
	}
	if deps.ContentRepo == nil {
		t.Error("expected ContentRepo to be set")
	}
	if deps.HotspotRepo == nil {
		t.Error("expected HotspotRepo to be set")
	}
	if deps.SourceRepo == nil {
		t.Error("expected SourceRepo to be set")
	}
	if deps.DashScope == nil {
		t.Error("expected DashScope to be set")
	}
	if deps.ScoreRepo == nil {
		t.Error("expected ScoreRepo to be set")
	}
}

func TestNewDepsWithoutDashScopeKey(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey",
		RedisURL:    "redis://127.0.0.1:6379/0",
	}

	mockDB, _ := sql.Open("postgres", "postgres://invalid/nil")
	mockRedis := redis.NewClient("redis://127.0.0.1:1", redis.Options{})

	deps, err := NewDeps(cfg,
		WithDialer(&testDialer{}),
		WithDB(mockDB),
		WithRedisClient(mockRedis),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer deps.Close()

	// DashScope API key 为空时，DashScope 客户端仍会被创建（空 key）
	// 具体业务层决定是否需要 DashScope
	if deps.DashScope == nil {
		t.Error("expected DashScope client to be created even without API key")
	}
}

func TestDepsClose(t *testing.T) {
	cfg := config.Config{
		DatabaseURL: "postgres://hotkey:hotkey@localhost:5432/hotkey",
		RedisURL:    "redis://127.0.0.1:6379/0",
	}

	mockDB, _ := sql.Open("postgres", "postgres://invalid/nil")
	mockRedis := redis.NewClient("redis://127.0.0.1:1", redis.Options{})

	deps, err := NewDeps(cfg,
		WithDialer(&testDialer{}),
		WithDB(mockDB),
		WithRedisClient(mockRedis),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close 不应 panic
	if err := deps.Close(); err != nil {
		t.Fatalf("close returned error: %v", err)
	}
}
