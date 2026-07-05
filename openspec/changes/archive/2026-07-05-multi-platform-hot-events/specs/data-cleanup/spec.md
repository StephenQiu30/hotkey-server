## ADDED Requirements

### Requirement: Data cleanup job
The system SHALL provide a scheduled job that deletes expired data according to configurable retention policies.

#### Scenario: Job registration
- **WHEN** the CleanupJob registers with the runner
- **THEN** it SHALL use the job name `cleanup_data`
- **AND** it SHALL run at an interval of 1 hour by default

#### Scenario: Platform posts retention
- **WHEN** the cleanup job runs
- **THEN** it SHALL delete platform_posts older than the configured retention period (default 30 days)
- **AND** it SHALL NOT delete posts referenced by active HotEvents

#### Scenario: HotEvent archival
- **WHEN** the cleanup job runs
- **THEN** it SHALL set Status="archived" for HotEvents last_seen more than 7 days ago
- **AND** it SHALL NOT physically delete archived HotEvents (soft archival)

#### Scenario: Configurable retention
- **WHEN** the system starts
- **THEN** retention periods SHALL be configurable via environment variables:
  - `DATA_RETENTION_DAYS` (default: 30)
  - `HOT_EVENT_ARCHIVE_DAYS` (default: 7)

#### Scenario: Cleanup logging
- **WHEN** the cleanup job deletes or archives records
- **THEN** it SHALL log the count of affected records per operation
