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
	integration integration-scylladb integration-openbao clean \
	dev-up dev-down dev-init dev-reset dev-kill \
	k3d-up k3d-down k3d-install-deps k3d-kubeconfig \
	dev dev-setup tilt-up tilt-down dev-tools require-tilt

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
# dev backing services — Docker Compose services used by the local control plane.
# -----------------------------------------------------------------------------
## dev-up: start local ScyllaDB, OpenBao, and Keycloak backing services
dev-up:
	docker compose -f dev/docker-compose.yml up -d
	@echo "Backing services started. Run 'make dev-init' to seed data."

## dev-down: stop local backing services without deleting persisted data
dev-down:
	docker compose -f dev/docker-compose.yml down

## dev-kill: stop local backing services without deleting persisted data and delete k3d cluster cloudforge-dev
dev-kill: dev-down k3d-down

## dev-init: start backing services, initialize OpenBao, and apply ScyllaDB migrations
dev-init: dev-up
	@echo "Waiting for services to be ready..."
	@sleep 5
	dev/scripts/init-openbao.sh
	dev/scripts/init-scylladb.sh
	@echo "Dev environment initialized"

## dev-reset: stop backing services and delete all Docker Compose volumes
dev-reset: dev-down
	docker compose -f dev/docker-compose.yml down -v
	@echo "Dev environment reset (all data deleted)"

## dev: initialize the full local dev environment (backing services plus k3d dependencies)
dev: dev-setup

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

# -----------------------------------------------------------------------------
# k3d — Local Kubernetes host cluster (Task 19; requires k3d, kubectl, helm).
# -----------------------------------------------------------------------------
## k3d-up: create k3d cluster cloudforge-dev (idempotent; LB HTTP/HTTPS default 18080/18443; override CF_K3D_LB_HTTP_PORT / CF_K3D_LB_HTTPS_PORT)
k3d-up:
	bash dev/k8s/cluster-up.sh

## k3d-install-deps: install Cilium, Envoy Gateway, vCluster operator, cert-manager, CloudForge namespace (uses k3d kubeconfig for Helm; optional: make k3d-kubeconfig for your default kubectl)
k3d-install-deps: k3d-up
	bash dev/k8s/install-cilium.sh
	bash dev/k8s/install-envoy-gateway.sh
	bash dev/k8s/install-vcluster.sh
	bash dev/k8s/install-cert-manager.sh
	bash dev/k8s/setup-cloudforge-namespace.sh
	@echo "All k8s dependencies installed"

## k3d-down: delete k3d cluster cloudforge-dev (idempotent)
k3d-down:
	bash dev/k8s/cluster-down.sh

## k3d-kubeconfig: merge cloudforge-dev kubeconfig into your default kubeconfig
k3d-kubeconfig:
	k3d kubeconfig merge cloudforge-dev --kubeconfig-merge-default
	@echo "Merged cloudforge-dev kubeconfig into default"

## dev-setup: initialize backing services and install the local k3d dependencies
dev-setup:
	$(MAKE) dev-init
	$(MAKE) k3d-install-deps
	@echo "Dev environment ready. Run 'make tilt-up' to start services."

## tilt-up: start the Tilt local development loop
tilt-up: require-tilt
	tilt up

## tilt-down: stop Tilt-managed resources
tilt-down: require-tilt
	tilt down

require-tilt:
	@command -v tilt >/dev/null 2>&1 || { \
		echo "ERROR: Tilt is not installed or is not on PATH."; \
		echo "Install it, then rerun 'make tilt-up'."; \
		echo "macOS: brew install tilt-dev/tap/tilt"; \
		echo "Other platforms: https://docs.tilt.dev/install.html"; \
		exit 127; \
	}

## dev-tools: install Go dev tools and print manual tool install links
dev-tools:
	@echo "Installing dev tools..."
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
	@echo "Install the following tools manually if not present:"
	@echo "  k3d:     https://k3d.io/#installation"
	@echo "  tilt:    https://docs.tilt.dev/install.html"
	@echo "  helm:    https://helm.sh/docs/intro/install/"
	@echo "  kubectl: https://kubernetes.io/docs/tasks/tools/"
