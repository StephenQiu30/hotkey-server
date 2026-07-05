## ADDED Requirements

### Requirement: TrendingCollectorJob
The system SHALL provide a scheduled job that periodically fetches trending data from all configured platforms (weibo, zhihu, baidu).

#### Scenario: Job registration
- **WHEN** the TrendingCollectorJob registers with the runner
- **THEN** it SHALL use the job name `collect_trending`
- **AND** it SHALL run at an interval of 5 minutes by default

#### Scenario: Multi-platform collection
- **WHEN** the job runs
- **THEN** it SHALL iterate over all configured TrendingCollector implementations
- **AND** for each collector, call `FetchTrending(ctx)` to get trending items
- **AND** for each trending item, upsert a record into `platform_posts`
- **AND** if a platform returns an error, log it and continue with the next platform

#### Scenario: Deduplication within a run
- **WHEN** a platform returns the same trending item as the previous run
- **THEN** the upsert SHALL update the existing post record (no duplicate rows)

### Requirement: TrendingCollector registration
The TrendingCollectorJob SHALL accept a list of TrendingCollector implementations at construction time.

#### Scenario: Dynamic collector list
- **WHEN** creating the TrendingCollectorJob
- **THEN** callers pass `[]connector.TrendingCollector{weiboClient, zhihuClient, baiduClient}`
- **AND** adding or removing a platform requires no Job code changes
