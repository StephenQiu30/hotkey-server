.PHONY: test lint build validate validate-schema validate-arch smoke up down schema dev swagger ci

test:
	go test ./... -v -count=1

test-unit:
	go test ./internal/service/... ./internal/handler/... -v -count=1

test-integration:
	go test ./tests/integration/... -v -count=1 -tags=integration

test-all: test-unit test-integration

lint:
	go vet ./...

build:
	go build -o hotkey-server ./cmd/hotkey

validate: validate-schema validate-arch

validate-schema:
	bash scripts/validate-schema.sh

validate-arch:
	bash scripts/validate-architecture-boundaries.sh

smoke:
	bash scripts/smoke-api.sh

ci: lint build test validate-schema validate-arch smoke

up:
	bash scripts/start-local.sh

down:
	docker compose down

schema:
	bash scripts/apply-schema.sh

dev:
	bash scripts/dev.sh

swagger:
	swag init -g cmd/hotkey/main.go -o docs --parseInternal --ot go
	# Remove LeftDelim/RightDelim from generated docs.go (CLI/lib version mismatch workaround)
	sed -i '' '/LeftDelim:/d; /RightDelim:/d' docs/docs.go
