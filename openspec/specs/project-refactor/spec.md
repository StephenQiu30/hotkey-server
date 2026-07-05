# project-refactor Specification

## Purpose

定义项目重构规范，包含 connector 接口提取、job 包职责分离和 wiring 注册模式。

## Requirements

### Requirement: Connector interface extraction

`PlatformConnector` 接口（原定义于 `internal/jobs/poll_monitor.go`）SHALL 迁移至 `internal/connector/search.go`。

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

每个功能领域 SHALL 拥有自己的包位于 `internal/` 下，包含自己的 Job 类型，而非所有 job 集中在 `internal/jobs/`。

#### Scenario: New job package structure
- **WHEN** a new job type is created（collector, aggregator, cleanup）
- **THEN** it SHALL live in its own package: `internal/collector/`, `internal/aggregator/`, `internal/cleanup/`
- **AND** each package SHALL expose a `Register(r *jobs.Runner, db *gorm.DB)` function

### Requirement: Wiring registration pattern

`internal/app/worker_jobs.go` SHALL 使用注册器模式，每个 job 包调用自己的 Register 方法。

#### Scenario: Registration call
- **WHEN** `newJobRunner()` is called
- **THEN** it SHALL invoke each job package's `Register(runner, db)` method
- **AND** adding a new job SHALL require only adding a new `pkg.Register(runner, db)` line
- **AND** SHALL NOT require modifying existing registration logic

### Requirement: 现有 job 包保持稳定

以下包 SHALL NOT 被移动或重构：`internal/jobs/runner.go`, `internal/jobs/daily_scheduler.go`, `internal/topic/`, `internal/scoring/`, `internal/monitor/`, `internal/config/`, `internal/llm/`, `internal/content/`, `internal/digest/`。

#### Scenario: No relocation
- **WHEN** the project-refactor is complete
- **THEN** none of the files listed above SHALL have changed their package path
