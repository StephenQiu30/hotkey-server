## ADDED Requirements

### Requirement: Trending API endpoint
The system SHALL expose a GET `/api/v1/trending` endpoint returning current trending items from each platform.

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
The system SHALL expose a GET `/api/v1/hot-events` endpoint returning a paginated list of hot events.

#### Scenario: HotEvent list success
- **WHEN** a GET request is made to `/api/v1/hot-events`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain a `data` array and `meta` with `total` count

#### Scenario: HotEvent list filters
- **WHEN** a GET request is made to `/api/v1/hot-events?status=active&platform=multi&sort=heat_score&limit=20`
- **THEN** the response SHALL filter by status="active", platform="multi", sort by heat_score DESC, limit 20

### Requirement: HotEvent detail API endpoint
The system SHALL expose GET `/api/v1/hot-events/:id` returning a single hot event with full details.

#### Scenario: HotEvent detail success
- **WHEN** a GET request is made to `/api/v1/hot-events/1`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain the full HotEvent object with all fields

#### Scenario: HotEvent detail not found
- **WHEN** a GET request is made to `/api/v1/hot-events/99999`
- **THEN** the response SHALL have HTTP status 404
- **AND** the JSON body SHALL contain an error message

### Requirement: HotEvent posts API endpoint
The system SHALL expose GET `/api/v1/hot-events/:id/posts` returning posts associated with a hot event.

#### Scenario: HotEvent posts success
- **WHEN** a GET request is made to `/api/v1/hot-events/1/posts`
- **THEN** the response SHALL have HTTP status 200
- **AND** the JSON body SHALL contain a `data` array of posts from all associated platforms

#### Scenario: HotEvent posts not found
- **WHEN** a GET request is made to `/api/v1/hot-events/99999/posts`
- **THEN** the response SHALL have HTTP status 404
