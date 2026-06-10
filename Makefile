.PHONY: test validate run fmt build compose-up compose-down compose-logs compose-prod-up compose-prod-down compose-prod-logs e2e-up e2e-down e2e-test e2e-smoke

# --- Standard targets ---

validate:
	bash scripts/validate-repository.sh

test: validate
	go test ./...

run:
	go run ./cmd/hotkey-api

fmt:
	gofmt -w cmd internal

build:
	go build ./...

# --- Docker Compose（应用容器；PostgreSQL / Redis 使用本机服务）---

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down

compose-logs:
	docker compose logs -f

# --- Docker Compose 生产全栈（PostgreSQL / Redis / MinIO / n8n + 应用）---

compose-prod-up:
	docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build

compose-prod-down:
	docker compose -f docker-compose.prod.yml --env-file .env.prod down

compose-prod-logs:
	docker compose -f docker-compose.prod.yml --env-file .env.prod logs -f

# --- E2E targets ---

e2e-up:
	./scripts/e2e-setup.sh

e2e-down:
	./scripts/e2e-teardown.sh

e2e-test:
	HOTKEY_E2E_POSTGRES_ADDR=127.0.0.1:15432 \
	HOTKEY_E2E_REDIS_URL=redis://127.0.0.1:16379/0 \
	HOTKEY_E2E_SERVER_URL=http://127.0.0.1:18080 \
	go test -v -tags e2e -count=1 -timeout=60s ./tests/e2e/...

e2e-smoke:
	HOTKEY_E2E_POSTGRES_ADDR=127.0.0.1:15432 \
	HOTKEY_E2E_REDIS_URL=redis://127.0.0.1:16379/0 \
	HOTKEY_E2E_SERVER_URL=http://127.0.0.1:18080 \
	go test -v -tags e2e -count=1 -timeout=30s -run "TestHealthCheck|TestAllBehaviors" ./tests/e2e/...
