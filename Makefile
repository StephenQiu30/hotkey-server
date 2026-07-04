.PHONY: test lint build validate up down schema dev swagger swagger-validate

test:
	go test ./...

lint:
	go vet ./...

build:
	go build -o hotkey-server ./cmd/hotkey

validate:
	bash scripts/validate-repository.sh

up:
	bash scripts/start-local.sh

down:
	docker compose down

schema:
	bash scripts/apply-schema.sh

dev:
	bash scripts/dev.sh

swagger:
	go run github.com/swaggo/swag/cmd/swag@latest init -g main.go -d cmd/hotkey,internal/platform/http,internal/content,internal/topic,internal/trend -o docs --parseInternal

swagger-validate:
	bash scripts/validate-swagger.sh
