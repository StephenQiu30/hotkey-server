.PHONY: test lint build validate up down schema dev

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
