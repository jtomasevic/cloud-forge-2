# =============================================================================
# CloudForge — root Makefile
# =============================================================================
# This repository is a Go workspace (go.work): there is no top-level go.mod.
# Targets here loop over every module that participates in the workspace and
# run the same Go command in each directory. Use `make help` to see every
# documented target.
#
# Prerequisites:
#   - Go 1.26+ (see go.work)
#   - Optional: Docker, for integration tests that start containers locally
#
# Typical workflows:
#   make build          # compile all modules (default when you run `make`)
#   make test           # unit tests everywhere
#   make verify         # vet + test (handy before a push)
#   make migrate        # apply ScyllaDB schema (delegates to tools/migrations/Makefile)
#   make integration    # libs that speak to real backing services (Docker)
# =============================================================================

.DEFAULT_GOAL := all

# Every directory listed in go.work that has its own go.mod.
MODULES := \
	services/cf-router \
	services/cf-accounts \
	services/cf-provisioner \
	libs/cloudforge-core \
	libs/scylladb \
	libs/openbao \
	libs/clients/cf-accounts \
	libs/clients/cf-provisioner \
	tools/cf-cli \
	tools/migrations

.PHONY: all help build test lint fmt verify codegen tidy work-sync migrate \
	integration integration-scylladb integration-openbao clean

# -----------------------------------------------------------------------------
# all — Default target: compile every workspace module.
# -----------------------------------------------------------------------------
## all: same as build (default when you run `make` with no arguments)
all: build

# -----------------------------------------------------------------------------
# help — Show targets documented with the `## description` convention below.
# -----------------------------------------------------------------------------
## help: print this list of targets and short descriptions
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'

# -----------------------------------------------------------------------------
# build — Run `go build ./...` in each module; fails fast on first error.
# -----------------------------------------------------------------------------
## build: compile all workspace modules (`go build ./...` per module)
build:
	@for m in $(MODULES); do \
		echo "Building $$m..."; \
		(cd $$m && go build ./...) || exit 1; \
	done

# -----------------------------------------------------------------------------
# test — Unit tests in each module (no integration tags, no live services).
# -----------------------------------------------------------------------------
## test: run unit tests in every module (`go test -race -count=1 ./...`)
test:
	@for m in $(MODULES); do \
		echo "Testing $$m..."; \
		(cd $$m && go test -race -count=1 ./...) || exit 1; \
	done

# -----------------------------------------------------------------------------
# lint — Static analysis via `go vet` in each module.
# -----------------------------------------------------------------------------
## lint: run `go vet ./...` in every module
lint:
	@for m in $(MODULES); do \
		echo "Vetting $$m..."; \
		(cd $$m && go vet ./...) || exit 1; \
	done

# -----------------------------------------------------------------------------
# fmt — Apply standard Go formatting in each module.
# -----------------------------------------------------------------------------
## fmt: run `go fmt ./...` in every module (rewrites files in place)
fmt:
	@for m in $(MODULES); do \
		echo "Formatting $$m..."; \
		(cd $$m && go fmt ./...) || exit 1; \
	done

# -----------------------------------------------------------------------------
# verify — Fast local gate: vet plus unit tests (no Docker).
# -----------------------------------------------------------------------------
## verify: run lint then test (common pre-push check)
verify: lint test

# -----------------------------------------------------------------------------
# codegen — Run `go generate` where //go:generate directives exist.
# -----------------------------------------------------------------------------
## codegen: run `go generate ./...` in each module (no-op if nothing to generate)
codegen:
	@for m in $(MODULES); do \
		echo "Generating $$m..."; \
		(cd $$m && go generate ./... 2>/dev/null || true); \
	done

# -----------------------------------------------------------------------------
# tidy — Sync module graphs after dependency or replace changes.
# -----------------------------------------------------------------------------
## tidy: run `go work sync` at the repo root, then `go mod tidy` in each module
tidy:
	go work sync
	@for m in $(MODULES); do \
		echo "go mod tidy $$m..."; \
		(cd $$m && go mod tidy) || exit 1; \
	done

# -----------------------------------------------------------------------------
# work-sync — Refresh workspace metadata only (lighter than full tidy).
# -----------------------------------------------------------------------------
## work-sync: run `go work sync` at the repository root
work-sync:
	go work sync

# -----------------------------------------------------------------------------
# migrate — Thin wrapper: variables and recipes live in tools/migrations/Makefile.
# -----------------------------------------------------------------------------
## migrate: apply ScyllaDB CQL migrations (see tools/migrations/Makefile; pass HOSTS=… KEYSPACE=… SCRIPTS_DIR=…)
migrate:
	@$(MAKE) -C tools/migrations migrate

# -----------------------------------------------------------------------------
# integration — Delegates to library Makefiles that start real dependencies.
# -----------------------------------------------------------------------------
## integration-scylladb: ScyllaDB integration tests (testcontainers or SCYLLADB_HOST)
integration-scylladb:
	$(MAKE) -C libs/scylladb integration-test

## integration-openbao: OpenBao integration tests (testcontainers or OPENBAO_* env)
integration-openbao:
	$(MAKE) -C libs/openbao integration-test

## integration: run ScyllaDB and OpenBao integration test suites (needs Docker)
integration: integration-scylladb integration-openbao

# -----------------------------------------------------------------------------
# clean — Remove known coverage artefacts from libs that write them locally.
# -----------------------------------------------------------------------------
## clean: delete coverage.out files under libs/scylladb and libs/openbao
clean:
	rm -f libs/scylladb/coverage.out libs/scylladb/integration_coverage.out
	rm -f libs/openbao/coverage.out libs/openbao/integration_coverage.out
