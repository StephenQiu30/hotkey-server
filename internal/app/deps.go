package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/dashscope"
	"github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/contentrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/hotspotrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/scorerepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/sourcerepo"
)

var (
	errDBRequired    = errors.New("数据库连接不可用")
	errRedisRequired = errors.New("Redis 连接不可用")
)

// DepsDialer 抽象 DB/Redis 连接检测，便于测试时注入 mock。
type DepsDialer interface {
	PingDB(ctx context.Context) error
	PingRedis(ctx context.Context) error
}

// Deps 持有 runtime、worker、scheduler 共享的核心依赖。
// 后续任务可直接复用，无需重复拼装基础设施客户端和 repository。
type Deps struct {
	Cfg         config.Config
	DB          *sql.DB
	RedisClient *redis.Client
	JobQueue    *queue.RedisQueue
	ContentRepo *contentrepo.Repository
	HotspotRepo *hotspotrepo.Repository
	ScoreRepo   *scorerepo.Repository
	SourceRepo  *sourcerepo.Repository
	DashScope   *dashscope.Client
}

// DepsOption 用于自定义依赖初始化行为（测试注入等）。
type DepsOption func(*depsOptions)

type depsOptions struct {
	dialer      DepsDialer
	db          *sql.DB
	redisClient *redis.Client
}

// WithDialer 注入自定义的连接检测器（测试用）。
func WithDialer(d DepsDialer) DepsOption {
	return func(o *depsOptions) { o.dialer = d }
}

// WithDB 注入已有的数据库连接（测试用，跳过 sql.Open）。
func WithDB(db *sql.DB) DepsOption {
	return func(o *depsOptions) { o.db = db }
}

// WithRedisClient 注入已有的 Redis 客户端（测试用，跳过 redis.NewClient）。
func WithRedisClient(c *redis.Client) DepsOption {
	return func(o *depsOptions) { o.redisClient = c }
}

// defaultDialer 使用真实的 DB 和 Redis 进行连接检测。
type defaultDialer struct {
	db    *sql.DB
	redis *redis.Client
}

func (d *defaultDialer) PingDB(ctx context.Context) error    { return d.db.PingContext(ctx) }
func (d *defaultDialer) PingRedis(ctx context.Context) error { return d.redis.Ping(ctx) }

// NewDeps 从配置统一初始化所有共享依赖。
// 返回的 Deps 包含 DB、Redis、Queue、Repository 和基础设施客户端。
// 调用方应在程序退出时调用 deps.Close() 释放资源。
func NewDeps(cfg config.Config, opts ...DepsOption) (*Deps, error) {
	o := &depsOptions{}
	for _, opt := range opts {
		opt(o)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Database ---
	db := o.db
	if db == nil {
		var err error
		db, err = sql.Open("postgres", cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("打开数据库失败: %w", err)
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(25)
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	// --- Redis ---
	redisClient := o.redisClient
	if redisClient == nil {
		redisClient = redis.NewClient(cfg.RedisURL, redis.Options{})
	}

	// --- 连接检测 ---
	dialer := o.dialer
	if dialer == nil {
		dialer = &defaultDialer{db: db, redis: redisClient}
	}

	if err := dialer.PingDB(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", errDBRequired, err)
	}
	if err := dialer.PingRedis(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", errRedisRequired, err)
	}

	// --- Repositories ---
	contentRepo := contentrepo.New(db)
	hotspotRepo := hotspotrepo.New(db)
	scoreRepo := scorerepo.New(db)
	sourceRepo := sourcerepo.New(db)

	// --- Infrastructure Providers ---
	dashScopeClient := dashscope.New(cfg.DashScopeAPIKey)

	// --- Queue ---
	jobQueue := queue.NewRedisQueue(redisClient, queue.RedisQueueOptions{
		QueueName: "hotkey:jobs:pending",
	})

	return &Deps{
		Cfg:         cfg,
		DB:          db,
		RedisClient: redisClient,
		JobQueue:    jobQueue,
		ContentRepo: contentRepo,
		HotspotRepo: hotspotRepo,
		ScoreRepo:   scoreRepo,
		SourceRepo:  sourceRepo,
		DashScope:   dashScopeClient,
	}, nil
}

// Close 释放依赖持有的资源。
func (d *Deps) Close() error {
	if d.DB != nil {
		return d.DB.Close()
	}
	return nil
}
