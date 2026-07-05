## ADDED Requirements

### Requirement: Connector interface extraction
The `PlatformConnector` interface currently defined in `internal/jobs/poll_monitor.go` SHALL be migrated to `internal/connector/search.go`.

#### Scenario: Interface relocation
- **WHEN** the migration is applied
- **THEN** `internal/connector/search.go` SHALL define the `Searcher` interface
- **AND** `internal/jobs/poll_monitor.go` SHALL remove the old `PlatformConnector` definition
- **AND** `internal/jobs/adapters.go` SHALL import and use the new `connector.Searcher` interface

#### Scenario: No behavioral change
- **WHEN** the migration is complete
- **THEN** all existing test cases SHALL pass without modification
- **AND** the binary SHALL build and start without errors

### Requirement: Job package responsibility separation
Each functional area SHALL have its own package under `internal/` with its own Job type, rather than all jobs residing in `internal/jobs/`.

#### Scenario: New job package structure
- **WHEN** a new job type is created (collector, aggregator, cleanup)
- **THEN** it SHALL live in its own package: `internal/collector/`, `internal/aggregator/`, `internal/cleanup/`
- **AND** each package SHALL expose a `Register(r *jobs.Runner, db *gorm.DB)` function

### Requirement: Wiring registration pattern
The `internal/app/worker_jobs.go` SHALL use a registration pattern where each job package calls its own Register method, rather than centralized instantiation.

#### Scenario: Registration call
- **WHEN** `newJobRunner()` is called
- **THEN** it SHALL invoke each job package's `Register(runner, db)` method
- **AND** adding a new job SHALL require only adding a new `pkg.Register(runner, db)` line
- **AND** SHALL NOT require modifying existing registration logic

### Requirement: Existing job packages remain stable
The following packages SHALL NOT be moved or restructured: `internal/jobs/runner.go`, `internal/jobs/daily_scheduler.go`, `internal/topic/`, `internal/scoring/`, `internal/monitor/`, `internal/config/`, `internal/llm/`, `internal/content/`, `internal/digest/`.

#### Scenario: No relocation
- **WHEN** the project-refactor is complete
- **THEN** none of the files listed above SHALL have changed their package path
