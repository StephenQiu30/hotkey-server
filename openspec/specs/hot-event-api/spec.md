# hot-event-api Specification

## Purpose

定义热点事件 REST API 端点规范，提供榜单数据和热点事件的查询接口。

## Requirements

### Requirement: Trending API endpoint

系统 SHALL 暴露 GET `/api/v1/trending` 端点，返回各平台当前榜单数据。

#### Scenario: Trending list success
- **WHEN** a GET request is made to `/api/v1/trending`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain a `data` array of trending items with platform, title, rank, heat, url fields
- **AND** each item SHALL be the latest snapshot from its platform

#### Scenario: Trending platform filter
- **WHEN** a GET request is made to `/api/v1/trending?platform=weibo`
- **THEN** the response SHALL contain only trending items from weibo

#### Scenario: Trending limit
- **WHEN** a GET request is made to `/api/v1/trending?limit=10`
- **THEN** the response SHALL contain at most 10 items per platform

### Requirement: HotEvent list API endpoint

系统 SHALL 暴露 GET `/api/v1/hot-events` 端点，返回分页热点事件列表。

#### Scenario: HotEvent list success
- **WHEN** a GET request is made to `/api/v1/hot-events`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain a `data` array and `meta` with `total` count

#### Scenario: HotEvent list filters
- **WHEN** a GET request is made to `/api/v1/hot-events?status=active&platform=multi&sort=heat_score&limit=20`
- **THEN** the response SHALL filter by status="active", platform="multi", sort by heat_score DESC, limit 20

### Requirement: HotEvent detail API endpoint

系统 SHALL 暴露 GET `/api/v1/hot-events/:id` 端点，返回单个热点事件详情。

#### Scenario: HotEvent detail success
- **WHEN** a GET request is made to `/api/v1/hot-events/1`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain the full HotEvent object with all fields

#### Scenario: HotEvent detail not found
- **WHEN** a GET request is made to `/api/v1/hot-events/99999`
- **THEN** the response SHALL have HTTP status 404
- **AND** the JSON body SHALL contain an error message

### Requirement: HotEvent posts API endpoint

系统 SHALL 暴露 GET `/api/v1/hot-events/:id/posts` 端点，返回与热点事件关联的帖子。

#### Scenario: HotEvent posts success
- **WHEN** a GET request is made to `/api/v1/hot-events/1/posts`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain a `data` array of posts from all associated platforms

#### Scenario: HotEvent posts not found
- **WHEN** a GET request is made to `/api/v1/hot-events/99999/posts`
- **THEN** the response SHALL have HTTP status 404
