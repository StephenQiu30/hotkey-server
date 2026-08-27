package rss

import (
	"fmt"
	"time"
)

const ResourceLimitProfileVersion = "rss-resource-limits-v1"
const maxResponseBodyBytes = 4 << 20

// ResourceLimitProfile freezes every RSS resource dimension independently.
// Source configuration may lower page/read limits but can never widen these
// profile ceilings.
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
		MaxPages: 5, MaxItems: 100, MaxCumulativeResponseBytes: maxResponseBodyBytes,
		MaxRetries: 2, DailyRequestQuota: 1440,
	}
}

func (profile ResourceLimitProfile) Validate() error {
	if profile.Version != ResourceLimitProfileVersion || profile.ConnectTimeout <= 0 || profile.ReadTimeout <= 0 ||
		profile.WallClockTimeout <= profile.ConnectTimeout || profile.WallClockTimeout <= profile.ReadTimeout ||
		profile.MaxPages < 1 || profile.MaxPages > 20 || profile.MaxItems < 1 || profile.MaxItems > 1000 ||
		profile.MaxCumulativeResponseBytes < 1 || profile.MaxCumulativeResponseBytes > 64<<20 ||
		profile.MaxRetries < 0 || profile.MaxRetries > 8 || profile.DailyRequestQuota < 1 || profile.DailyRequestQuota > 1_000_000 {
		return fmt.Errorf("invalid RSS resource limit profile")
	}
	return nil
}
