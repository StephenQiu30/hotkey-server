.PHONY: test lint build

test:
	go test ./internal/alert/... ./internal/notify/... ./internal/jobs/...

lint:
	go vet ./internal/alert/... ./internal/notify/... ./internal/jobs/...

build:
	go build ./...
