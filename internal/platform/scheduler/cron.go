// Package scheduler contains due-time and stable-key rules. It does not run
// business work in the scheduler process; it only asks the queue to enqueue a
// bounded job.
package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type DueSource struct {
	ID        int64
	NextPoll  time.Time
	QueryHash string
}

func IsDue(source DueSource, now time.Time) bool {
	return source.ID > 0 && !source.NextPoll.IsZero() && !source.NextPoll.After(now)
}

func UniqueKey(kind string, entityID int64, version int64, windowStart, windowEnd time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s:%s", kind, entityID, version, windowStart.UTC().Format(time.RFC3339Nano), windowEnd.UTC().Format(time.RFC3339Nano))))
	return hex.EncodeToString(sum[:])
}
