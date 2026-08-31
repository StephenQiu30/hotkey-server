package x

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const ResourceLimitProfileVersion = "x-api-resource-limits-v1"

var errResponseByteLimit = errors.New("x cumulative response byte limit exceeded")

// ResourceLimitProfile applies to both Recent Search and Post Lookup. One
// connector invocation consumes one logical API page, while retries and
// redirects remain separate physical requests for durable quota accounting.
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
		MaxPages: 1, MaxItems: 100, MaxCumulativeResponseBytes: 4 << 20,
		MaxRetries: 2, DailyRequestQuota: 4500,
	}
}

func (profile ResourceLimitProfile) Validate() error {
	if profile.Version != ResourceLimitProfileVersion || profile.ConnectTimeout <= 0 || profile.ReadTimeout <= 0 ||
		profile.WallClockTimeout <= profile.ConnectTimeout || profile.WallClockTimeout <= profile.ReadTimeout ||
		profile.MaxPages != 1 || profile.MaxItems < 10 || profile.MaxItems > 100 ||
		profile.MaxCumulativeResponseBytes < 1 || profile.MaxCumulativeResponseBytes > 64<<20 ||
		profile.MaxRetries < 0 || profile.MaxRetries > 8 || profile.DailyRequestQuota < 1 || profile.DailyRequestQuota > 1_000_000 {
		return fmt.Errorf("invalid X resource limit profile")
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
