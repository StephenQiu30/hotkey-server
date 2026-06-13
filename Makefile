.PHONY: test lint build validate

test:
	go test ./...

lint:
	go vet ./...

build:
	go build ./...

validate:
	bash scripts/validate-repository.sh
