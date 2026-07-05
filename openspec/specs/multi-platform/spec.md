# multi-platform Specification

## Purpose

定义多平台连接器抽象层，使系统能统一接入多个外部数据源（X、微博、知乎、百度）的热点数据。

## Requirements

### Requirement: Connector interface abstraction

系统 SHALL 定义 `Searcher` 和 `TrendingCollector` 接口在 `internal/connector/` 包中，与 job 调度解耦。

#### Scenario: Searcher interface definition
- **WHEN** the system imports the connector package
- **THEN** it SHALL have access to a `Searcher` interface with `SearchPosts(ctx, query, cursor) ([]SearchResult, string, error)` and `Name() string`

#### Scenario: TrendingCollector interface definition
- **WHEN** the system imports the connector package
- **THEN** it SHALL have access to a `TrendingCollector` interface with `FetchTrending(ctx) ([]TrendingItem, error)` and `Name() string`

#### Scenario: Shared types availability
- **WHEN** the connector package is imported
- **THEN** the types `SearchResult`, `TrendingItem`, and `PostResult` SHALL be available for use by all platform adapters

### Requirement: 平台客户端位于 internal/platform/

每个外部数据源 SHALL 有自己的包位于 `internal/platform/<name>/`，实现 connector 接口。

#### Scenario: New platform client location
- **WHEN** a new platform (e.g., weibo, zhihu, baidu) is added
- **THEN** its client code SHALL live in `internal/platform/<name>/client.go` and import only `internal/connector/` types
- **AND** SHALL NOT import `internal/jobs/`, `internal/app/`, or any HTTP handler packages

### Requirement: 平台客户端使用 HTTP 直连

所有平台客户端 SHALL 直接发起 HTTP 请求，不使用第三方 SDK。

#### Scenario: HTTP client usage
- **WHEN** a platform client makes a request
- **THEN** it SHALL use `net/http.Client` with a configurable timeout
- **AND** the response parsing SHALL be done with `encoding/json` or `net/html` as needed

#### Scenario: Failure isolation
- **WHEN** one platform client fails (network error, parse error)
- **THEN** the error SHALL be returned to the caller without affecting other platform clients
- **AND** the system SHALL continue operating with other platforms
