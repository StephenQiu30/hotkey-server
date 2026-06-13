.PHONY: test lint build validate up down schema dev dev-worker

test:
	go test ./...

lint:
	go vet ./...

build:
	go build ./...

validate:
	bash scripts/validate-repository.sh

up:
	bash scripts/start-local.sh

down:
	docker compose down

schema:
	bash scripts/apply-schema.sh

dev:
	bash scripts/dev-api.sh

dev-worker:
	bash scripts/dev-worker.sh
