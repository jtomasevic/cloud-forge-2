.PHONY: build test coverage coverage-html integration-test lint tidy clean

# Unit-testable packages (no live ScyllaDB required).
SOURCE_PKGS := ./pkg/client ./pkg/migrate

# All packages that have integration tests.
INTEGRATION_PKGS := ./pkg/client ./pkg/migrate

## build: compile the module and all sub-packages
build:
	go build ./...

## test: run unit tests (no live ScyllaDB required)
test:
	go test $(SOURCE_PKGS) -race -count=1

## coverage: run tests and print per-function coverage to stdout
coverage:
	go test $(SOURCE_PKGS) -coverprofile=coverage.out -covermode=atomic
	@go tool cover -func=coverage.out
	@echo ""
	@echo "Overall coverage:"
	@go tool cover -func=coverage.out | grep "^total:"

## coverage-html: open an HTML coverage report in the browser
coverage-html: coverage
	go tool cover -html=coverage.out

## integration-test: run integration tests (starts ScyllaDB via testcontainers-go)
## Set SCYLLADB_HOST=host:port to skip container startup and use an existing instance.
integration-test:
	go test $(INTEGRATION_PKGS) \
		-tags integration \
		-race -count=1 -v \
		-timeout 5m

## lint: run go vet (add golangci-lint if available)
lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, skipping extended lint"; \
	fi

## tidy: sync go.mod and go.sum
tidy:
	go mod tidy

## clean: remove build artefacts
clean:
	rm -f coverage.out integration_coverage.out

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
