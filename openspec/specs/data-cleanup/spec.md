# data-cleanup Specification

## Purpose

定义数据清理 Job 的行为规范，按可配置的保留策略删除过期数据。

## Requirements

### Requirement: Data cleanup job

系统 SHALL 提供定时 Job，根据可配置的保留策略删除过期数据。

#### Scenario: Job registration
- **WHEN** the CleanupJob registers with the runner
- **THEN** it SHALL use the job name `cleanup_data`
- **AND** it SHALL run at an interval of 1 hour by default

#### Scenario: Platform posts retention
- **WHEN** the cleanup job runs
- **THEN** it SHALL delete platform_posts older than the configured retention period（默认 30 天）
- **AND** it SHALL NOT delete posts referenced by active HotEvents

#### Scenario: HotEvent archival
- **WHEN** the cleanup job runs
- **THEN** it SHALL set Status="archived" for HotEvents last_seen more than 7 days ago
- **AND** it SHALL NOT physically delete archived HotEvents（软归档）

#### Scenario: Configurable retention
- **WHEN** the system starts
- **THEN** retention periods SHALL be configurable via environment variables:
  - `DATA_RETENTION_DAYS`（默认: 30）
  - `HOT_EVENT_ARCHIVE_DAYS`（默认: 7）

#### Scenario: Cleanup logging
- **WHEN** the cleanup job deletes or archives records
- **THEN** it SHALL log the count of affected records per operation
