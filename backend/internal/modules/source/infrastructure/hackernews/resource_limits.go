package hackernews

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const ResourceLimitProfileVersion = "hacker-news-resource-limits-v1"

var errResponseByteLimit = errors.New("hacker news cumulative response byte limit exceeded")

// ResourceLimitProfile freezes every Hacker News resource dimension. One
// Fetch call consumes one logical page; the page may fan out into bounded
// official Item requests, all of which share bytes, wall-clock and quota.
type ResourceLimitProfile struct {
	Version                    string
	ConnectTimeout             time.Duration
	ReadTimeout                time.Duration
	WallClockTimeout           time.Duration
	MaxPages                   int
	MaxItems                   int
	MaxCumulativeResponseBytes int64
	MaxRetries                 int
	DailyRequestQuota          int64
}

func DefaultResourceLimitProfile() ResourceLimitProfile {
	return ResourceLimitProfile{
		Version:        ResourceLimitProfileVersion,
		ConnectTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WallClockTimeout: 60 * time.Second,
		MaxPages: 1, MaxItems: 100, MaxCumulativeResponseBytes: 8 << 20,
		MaxRetries: 2, DailyRequestQuota: 10_000,
	}
}

func (profile ResourceLimitProfile) Validate() error {
	if profile.Version != ResourceLimitProfileVersion || profile.ConnectTimeout <= 0 || profile.ReadTimeout <= 0 ||
		profile.WallClockTimeout <= profile.ConnectTimeout || profile.WallClockTimeout <= profile.ReadTimeout ||
		profile.MaxPages != 1 || profile.MaxItems < 1 || profile.MaxItems > 1000 ||
		profile.MaxCumulativeResponseBytes < 1 || profile.MaxCumulativeResponseBytes > 64<<20 ||
		profile.MaxRetries < 0 || profile.MaxRetries > 8 || profile.DailyRequestQuota < 1 || profile.DailyRequestQuota > 1_000_000 {
		return fmt.Errorf("invalid Hacker News resource limit profile")
	}
	return nil
}

type responseByteBudget struct {
	mu        sync.Mutex
	remaining int64
}

func newResponseByteBudget(limit int64) *responseByteBudget {
	return &responseByteBudget{remaining: limit}
}

func (budget *responseByteBudget) read(body io.Reader) ([]byte, error) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.remaining <= 0 {
		return nil, errResponseByteLimit
	}
	payload, err := io.ReadAll(io.LimitReader(body, budget.remaining+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > budget.remaining {
		return nil, errResponseByteLimit
	}
	budget.remaining -= int64(len(payload))
	return payload, nil
}

func (budget *responseByteBudget) readResponse(ctx context.Context, body io.Reader, contentEncoding string) ([]byte, error) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.remaining <= 0 {
		return nil, errResponseByteLimit
	}
	payload, err := sourcenet.ReadBoundedResponse(ctx, body, contentEncoding, sourcenet.DefaultCompressionLimits(budget.remaining))
	if err != nil {
		return nil, err
	}
	budget.remaining -= int64(len(payload))
	return payload, nil
}
