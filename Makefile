.PHONY: test lint build validate validate-arch validate-repository ci clean

GO ?= go

test:
	$(GO) test ./... -count=1

lint:
	$(GO) vet ./...

build:
	$(GO) build -o hotkey-server ./cmd/hotkey

validate: validate-arch validate-repository

validate-arch:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/validate_architecture.ps1

validate-repository:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/validate_repository.ps1

ci: lint test build validate

clean:
	powershell -NoProfile -Command "Remove-Item -LiteralPath hotkey-server -Force -ErrorAction SilentlyContinue"
