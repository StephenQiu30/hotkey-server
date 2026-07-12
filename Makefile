.PHONY: test lint build validate validate-schema validate-arch smoke up down schema schema-rebuild dev ci

test:
	go test ./... -v -count=1

lint:
	go vet ./...

build:
	go build -o hotkey-server ./cmd/hotkey

validate: validate-schema validate-arch

validate-schema:
	bash scripts/validate-schema.sh

validate-arch:
	bash scripts/validate-architecture-boundaries.sh

swagger:
	swag init -g cmd/hotkey/main.go --output docs/ --parseInternal --parseDependency --parseDepth 2

smoke:
	bash scripts/smoke-api.sh

ci: lint build test validate-schema validate-arch smoke

up:
	bash scripts/start-local.sh

down:
	docker compose down

schema:
	bash scripts/apply-schema.sh

schema-rebuild:
	bash db/tables/build.sh

dev:
	bash scripts/dev.sh
