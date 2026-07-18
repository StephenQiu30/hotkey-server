.PHONY: test lint build validate validate-arch validate-repository ci smoke clean schema-verify database-runtime-verify openapi openapi-validate openapi-check

GO ?= go

test:
	test -n "$$HOTKEY_TEST_DSN"
	GO=$(GO) $(GO) run ./test/runner test ./... -count=1

lint:
	GO=$(GO) $(GO) run ./test/runner vet ./...

build:
	$(GO) build -o hotkey ./cmd/hotkey

validate: validate-arch validate-repository

validate-arch:
	sh test/tools/validate-architecture.sh

validate-repository:
	sh test/tools/validate-repository.sh

schema-verify:
	sh test/tools/verify-schema.sh

database-runtime-verify:
	sh test/tools/verify-database-runtime.sh

openapi:
	$(GO) run github.com/swaggo/swag/cmd/swag init --generalInfo cmd/hotkey/main.go --parseInternal --output docs/openapi --outputTypes go,json --packageName openapi

openapi-validate:
	$(GO) test ./test/architecture -run 'Test(OpenAPIContract|GeneratedOpenAPIRegistryMatchesCommittedArtifact)$$' -count=1

openapi-check: openapi openapi-validate
	git diff --exit-code -- docs/openapi/docs.go docs/openapi/swagger.json

ci: openapi-check lint database-runtime-verify test build validate schema-verify

smoke: openapi-validate build
	$(MAKE) clean

clean:
	rm -f hotkey
