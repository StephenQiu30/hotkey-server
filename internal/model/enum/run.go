package enum

// RunType defines the type of a monitor run.
type RunType string

const (
	RunTypePoll           RunType = "poll"
	RunTypeManual         RunType = "manual"
	RunTypeBackfill       RunType = "backfill"
	RunTypeHourlyAggregate RunType = "hourly_aggregate"
)

// RunStatus defines monitor run lifecycle states.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

// KnowledgeRunType defines the run_type for knowledge_runs table.
type KnowledgeRunType string

const (
	KnowledgeRunTypeDailyDigest KnowledgeRunType = "daily-digest"
	KnowledgeRunTypeHourly      KnowledgeRunType = "hourly"
)

// KnowledgeRunStatus defines the status for knowledge_runs table.
type KnowledgeRunStatus string

const (
	KnowledgeRunStatusPending   KnowledgeRunStatus = "pending"
	KnowledgeRunStatusRunning   KnowledgeRunStatus = "running"
	KnowledgeRunStatusCompleted KnowledgeRunStatus = "completed"
	KnowledgeRunStatusFailed    KnowledgeRunStatus = "failed"
)
