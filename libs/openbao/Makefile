.PHONY: build test coverage coverage-html generate lint tidy clean

# Source packages to measure (excludes generated mock code)
SOURCE_PKGS := ./pkg/client

## build: compile the module and all sub-packages
build:
	go build ./...

## test: run unit tests (no live OpenBao required)
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

## generate: regenerate gomock mocks from the current interface definitions
generate:
	go generate ./...

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
	rm -f coverage.out

## help: list available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
