# E2E Test Fixtures

## seed.sql

PostgreSQL seed data loaded automatically by `docker-compose.e2e.yml`.
Contains minimal test tenant, users, keywords, sources, contents, and events.

## git-remote/

Bare Git repository used as a test remote for Git-related E2E tests.
Initialized with `git init --bare`. Tests can clone/push to this path.
