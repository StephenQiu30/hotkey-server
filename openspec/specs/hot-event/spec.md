# hot-event Specification

## Purpose

定义热点事件（HotEvent）领域实体、存储接口和业务服务，支持跨平台事件聚合后的统一管理和查询。

## Requirements

### Requirement: HotEvent entity

系统 SHALL 在 `internal/hotevent/model.go` 中定义 HotEvent 实体，字段含：ID, Name, HeatScore, Platform, Trend, FirstSeenAt, LastSeenAt, PeakAt, TopicIDs, PostIDs, Summary, Category, Status, CreatedAt, UpdatedAt。

#### Scenario: HotEvent creation
- **WHEN** a new HotEvent is created from matching Topic and TrendingItem data
- **THEN** it SHALL have a unique ID, auto-generated Name, computed HeatScore, and Status="active"

#### Scenario: HotEvent status lifecycle
- **WHEN** a HotEvent has not been updated for 7 days
- **THEN** its Status SHALL be set to "archived" by the cleanup job

### Requirement: HotEvent repository

系统 SHALL 在 `internal/hotevent/repository.go` 中提供 repository 接口，包含 Create, GetByID, List（带过滤器）, Update, Archive 操作。

#### Scenario: List with platform filter
- **WHEN** the List method is called with platform="multi"
- **THEN** it SHALL return HotEvents whose Platform field contains "multi", ordered by HeatScore DESC

#### Scenario: List with status filter
- **WHEN** the List method is called with status="active"
- **THEN** it SHALL return only HotEvents with Status="active"

#### Scenario: GetByID not found
- **WHEN** GetByID is called with a non-existent ID
- **THEN** it SHALL return a sentinel error comparable to `hotevent.ErrNotFound`

### Requirement: HotEvent service

系统 SHALL 在 `internal/hotevent/service.go` 中提供 HeatScore 计算和生命周期管理服务。

#### Scenario: HeatScore computation
- **WHEN** computing HeatScore for a HotEvent
- **THEN** it SHALL use the formula: `w_platform * Σ(post_heat * decay_factor)`
- **AND** platform weights SHALL be: X=1.0, weibo=1.0, zhihu=0.8, baidu=0.7
- **AND** decay_factor SHALL follow the same time-decay function as existing scoring.Service

### Requirement: Database tables

系统 SHALL 创建 `hot_events` 和 `hot_event_platforms` 两张数据库表。

#### Scenario: hot_events table schema
- **WHEN** the migration runs
- **THEN** the `hot_events` table SHALL have columns: id (BIGSERIAL PK), name, heat_score, platform, trend, first_seen_at, last_seen_at, peak_at, topic_ids (BIGINT[]), post_ids (BIGINT[]), summary, category, status, created_at, updated_at

#### Scenario: hot_event_platforms table schema
- **WHEN** the migration runs
- **THEN** the `hot_event_platforms` table SHALL have columns: hot_event_id, platform, rank, title, url, heat, updated_at
- **AND** the PRIMARY KEY SHALL be (hot_event_id, platform)
- **AND** hot_event_id SHALL reference hot_events(id) with CASCADE delete

#### Scenario: Existing table compatibility
- **WHEN** inserting posts from new platforms
- **THEN** the existing `platform_posts` table SHALL be used without schema changes（`platform` 列和 `(platform, platform_post_id)` 唯一约束已支持多平台）
