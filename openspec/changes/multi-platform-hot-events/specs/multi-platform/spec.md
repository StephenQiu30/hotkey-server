## ADDED Requirements

### Requirement: Connector interface abstraction
The system SHALL define Searcher and TrendingCollector interfaces in a dedicated `internal/connector/` package, decoupled from job scheduling.

#### Scenario: Searcher interface definition
- **WHEN** the system imports the connector package
- **THEN** it SHALL have access to a `Searcher` interface with `SearchPosts(ctx, query, cursor) ([]SearchResult, string, error)` and `Name() string`

#### Scenario: TrendingCollector interface definition
- **WHEN** the system imports the connector package
- **THEN** it SHALL have access to a `TrendingCollector` interface with `FetchTrending(ctx) ([]TrendingItem, error)` and `Name() string`

#### Scenario: Shared types availability
- **WHEN** the connector package is imported
- **THEN** the types `SearchResult`, `TrendingItem`, and `PostResult` SHALL be available for use by all platform adapters

### Requirement: Platform-specific clients reside in internal/platform/
Each external data source SHALL have its own package under `internal/platform/<name>/` implementing the connector interfaces.

#### Scenario: New platform client location
- **WHEN** a new platform (e.g., weibo, zhihu, baidu) is added
- **THEN** its client code SHALL live in `internal/platform/<name>/client.go` and import only `internal/connector/` types
- **AND** SHALL NOT import `internal/jobs/`, `internal/app/`, or any HTTP handler packages

### Requirement: Platform clients use HTTP direct calls
All platform clients SHALL make HTTP requests directly without third-party SDKs.

#### Scenario: HTTP client usage
- **WHEN** a platform client makes a request
- **THEN** it SHALL use `net/http.Client` with a configurable timeout
- **AND** the response parsing SHALL be done with `encoding/json` or `net/html` as needed

#### Scenario: Failure isolation
- **WHEN** one platform client fails (network error, parse error)
- **THEN** the error SHALL be returned to the caller without affecting other platform clients
- **AND** the system SHALL continue operating with other platforms
