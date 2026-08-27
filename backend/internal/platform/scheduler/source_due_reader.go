package scheduler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

// CollectionDueSource is the scheduler-safe projection of one or more
// immutable published Monitor targets sharing a source/signature/window.
// Cron never receives connector details and never mutates a checkpoint.
type CollectionDueSource struct {
	MonitorID          int64
	MonitorVersionID   int64
	CompiledProfileID  int64
	SourceConnectionID int64
	QuerySignature     string
	NextPollAt         time.Time
	CollectionInterval time.Duration
}

const ManualCollectionCooldown = 5 * time.Minute

func (source CollectionDueSource) Validate() error {
	if source.MonitorID <= 0 || source.MonitorVersionID <= 0 || source.CompiledProfileID <= 0 ||
		source.SourceConnectionID <= 0 || !validCollectionInputHash(source.QuerySignature) || source.NextPollAt.IsZero() {
		return fmt.Errorf("invalid collection due source")
	}
	if source.CollectionInterval < 5*time.Minute || source.CollectionInterval > 24*time.Hour || source.CollectionInterval%time.Minute != 0 {
		return fmt.Errorf("invalid collection collection interval")
	}
	return nil
}

// CollectionJobArgs is the bounded River contract for one published Monitor
// collection window. It contains immutable identities, the window and the
// published input hash only; query text, connector configuration, rights,
// credentials and checkpoints are always reread by the worker.
type CollectionJobArgs struct {
	MonitorID          int64     `json:"monitor_id"`
	MonitorVersionID   int64     `json:"monitor_version_id"`
	CompiledProfileID  int64     `json:"compiled_profile_id"`
	SourceConnectionID int64     `json:"source_connection_id"`
	WindowStart        time.Time `json:"window_start"`
	WindowEnd          time.Time `json:"window_end"`
	InputHash          string    `json:"input_hash"`
	TriggerType        string    `json:"trigger_type"`
}

func (args CollectionJobArgs) Validate() error {
	if args.MonitorID <= 0 || args.MonitorVersionID <= 0 || args.CompiledProfileID <= 0 || args.SourceConnectionID <= 0 ||
		args.WindowStart.IsZero() || args.WindowEnd.IsZero() || !args.WindowEnd.After(args.WindowStart) ||
		!validCollectionInputHash(args.InputHash) || (args.TriggerType != "schedule" && args.TriggerType != "manual") {
		return fmt.Errorf("invalid collection job args")
	}
	return nil
}

func EncodeCollectionJobArgs(args CollectionJobArgs) ([]byte, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode collection job args")
	}
	return encoded, nil
}

func DecodeCollectionJobArgs(encoded []byte) (CollectionJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args CollectionJobArgs
	if err := decoder.Decode(&args); err != nil {
		return args, fmt.Errorf("decode collection job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return args, fmt.Errorf("collection job args contain trailing data")
	}
	if err := args.Validate(); err != nil {
		return args, err
	}
	args.WindowStart = args.WindowStart.UTC()
	args.WindowEnd = args.WindowEnd.UTC()
	return args, nil
}

type CollectionDueReader interface {
	ListDueCollections(context.Context, time.Time) ([]CollectionDueSource, error)
}

type CollectionScheduler struct {
	reader CollectionDueReader
	store  Enqueuer
}

func NewCollectionScheduler(reader CollectionDueReader, store Enqueuer) *CollectionScheduler {
	return &CollectionScheduler{reader: reader, store: store}
}

// RunOnce scans only due published targets and submits collect_source jobs.
// It does not call a connector, create a collection run, or advance a
// checkpoint; those facts belong to the worker and Source application.
func (scheduler *CollectionScheduler) RunOnce(ctx context.Context, now time.Time) (int, error) {
	if scheduler == nil || scheduler.reader == nil || scheduler.store == nil || now.IsZero() {
		return 0, fmt.Errorf("collection scheduler is not initialized")
	}
	sources, err := scheduler.reader.ListDueCollections(ctx, now.UTC())
	if err != nil {
		return 0, err
	}
	created := 0
	for _, source := range sources {
		if err := source.Validate(); err != nil {
			return created, err
		}
		if !IsDue(DueSource{ID: source.SourceConnectionID, NextPoll: source.NextPollAt}, now) {
			return created, fmt.Errorf("collection due source is not due")
		}
		windowStart := source.NextPollAt.UTC()
		windowEnd := windowStart.Add(source.CollectionInterval)
		args := CollectionJobArgs{
			MonitorID: source.MonitorID, MonitorVersionID: source.MonitorVersionID,
			CompiledProfileID: source.CompiledProfileID, SourceConnectionID: source.SourceConnectionID,
			WindowStart: windowStart, WindowEnd: windowEnd, InputHash: source.QuerySignature, TriggerType: "schedule",
		}
		encoded, err := EncodeCollectionJobArgs(args)
		if err != nil {
			return created, err
		}
		_, wasCreated, err := scheduler.store.Enqueue(ctx, queue.Job{
			Kind:        queue.KindCollectSource,
			UniqueKey:   CollectionUniqueKey(source.MonitorID, source.MonitorVersionID, source.CompiledProfileID, source.SourceConnectionID, windowStart, windowEnd),
			DurableArgs: encoded,
			ScheduledAt: now.UTC(),
			MaxAttempts: 3,
			Priority:    1,
		})
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}

func (scheduler *CollectionScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("collection scheduler interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := scheduler.RunOnce(ctx, time.Now().UTC()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func CollectionUniqueKey(monitorID, monitorVersionID, compiledProfileID, sourceConnectionID int64, windowStart, windowEnd time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("collect_source:%d:%d:%d:%d:%s:%s", monitorID, monitorVersionID, compiledProfileID,
		sourceConnectionID, windowStart.UTC().Format(time.RFC3339Nano), windowEnd.UTC().Format(time.RFC3339Nano))))
	return hex.EncodeToString(sum[:])
}

func ManualCollectionUniqueKey(monitorID, monitorVersionID, compiledProfileID, sourceConnectionID int64, scheduledAt time.Time) string {
	bucket := scheduledAt.UTC().Truncate(ManualCollectionCooldown)
	sum := sha256.Sum256([]byte(fmt.Sprintf("collect_source:manual:%d:%d:%d:%d:%s", monitorID, monitorVersionID,
		compiledProfileID, sourceConnectionID, bucket.Format(time.RFC3339Nano))))
	return hex.EncodeToString(sum[:])
}

func ManualCollectionCooldownUntil(scheduledAt time.Time) time.Time {
	return scheduledAt.UTC().Truncate(ManualCollectionCooldown).Add(ManualCollectionCooldown)
}

func validCollectionInputHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}
