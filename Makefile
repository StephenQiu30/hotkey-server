.PHONY: test run fmt workflow-test

test:
	go test ./...
	python3 -m unittest discover -s tests

workflow-test:
	python3 -m unittest tests/test_workflow_contract.py

run:
	go run ./cmd/hotkey-api

fmt:
	gofmt -w cmd internal
