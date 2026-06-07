# E2E Test Fixtures

## seed.sql

PostgreSQL seed data loaded automatically by `docker-compose.e2e.yml`.
Contains minimal test tenant, users, keywords, sources, contents, and events.

## git-remote/

Bare Git repository created dynamically by `scripts/e2e-setup.sh` and
cleaned up by `scripts/e2e-teardown.sh`. Used as a test remote for
Git-related E2E tests. Not committed to the repository.
