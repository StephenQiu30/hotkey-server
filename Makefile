.PHONY: test lint build validate validate-arch validate-repository ci clean schema-verify database-runtime-verify

GO ?= go

test:
	test -n "$$HOTKEY_TEST_DSN"
	$(GO) test ./... -count=1

lint:
	$(GO) vet ./...

build:
	$(GO) build -o hotkey ./cmd/hotkey

validate: validate-arch validate-repository

validate-arch:
	sh scripts/validate-architecture.sh

validate-repository:
	sh scripts/validate-repository.sh

schema-verify:
	sh scripts/verify-schema.sh

database-runtime-verify:
	sh scripts/verify-database-runtime.sh

ci: lint database-runtime-verify test build validate schema-verify

clean:
	rm -f hotkey
