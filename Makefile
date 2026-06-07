.PHONY: test run fmt workflow-test build e2e-up e2e-down e2e-test e2e-smoke

# --- Standard targets ---

test:
	go test ./...
	python3 -m unittest discover -s tests

workflow-test:
	python3 -m unittest tests/test_workflow_contract.py

run:
	go run ./cmd/hotkey-api

fmt:
	gofmt -w cmd internal

build:
	go build ./...

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
	go test -v -tags e2e -count=1 -timeout=30s -run "TestHealthCheck|TestAllBehaviors" ./tests/e2e/...
