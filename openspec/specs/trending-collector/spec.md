# trending-collector Specification

## Purpose

定义多平台榜单采集 Job 的行为规范，定期从所有已配置平台获取热门榜单数据。

## Requirements

### Requirement: TrendingCollectorJob

系统 SHALL 提供定时 Job，定期从所有已配置平台（weibo, zhihu, baidu）获取榜单数据。

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
- **THEN** the upsert SHALL update the existing post record（无重复行）

### Requirement: TrendingCollector registration

TrendingCollectorJob SHALL 在构造时接受 `TrendingCollector` 实现列表。

#### Scenario: Dynamic collector list
- **WHEN** creating the TrendingCollectorJob
- **THEN** callers pass `[]connector.TrendingCollector{weiboClient, zhihuClient, baiduClient}`
- **AND** adding or removing a platform requires no Job code changes
