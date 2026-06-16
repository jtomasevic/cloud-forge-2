# =============================================================================
# tools/migrations — ScyllaDB schema migration CLI
# =============================================================================
# Applies idempotent `.cql` files from ./scripts in lexicographic order using
# libs/scylladb/pkg/migrate. Requires a reachable ScyllaDB CQL endpoint.
#
# From this directory:
#   make help      # list targets
#   make migrate   # apply scripts (needs Scylla on HOSTS)
#
# From repo root:
#   make -C tools/migrations migrate
#   make migrate   # if root Makefile delegates here
#
# Variables (override on the command line, e.g. `make migrate HOSTS=127.0.0.1:9042`):
#   HOSTS        — comma-separated contact points (default localhost:9042)
#   KEYSPACE     — keyspace name (default cloudforge)
#   SCRIPTS_DIR  — directory of .cql files, relative to this Makefile (default ./scripts)
# =============================================================================

.DEFAULT_GOAL := help

# CQL contact points (comma-separated). Matches tools/migrations/main.go --hosts default.
HOSTS ?= localhost:9042
# Target keyspace; bootstrap creates it if missing before applying scripts.
KEYSPACE ?= cloudforge
# Path to migration .cql files (relative to this directory).
SCRIPTS_DIR ?= ./scripts

.PHONY: help build migrate test tidy fmt vet

# -----------------------------------------------------------------------------
# help — List documented targets (lines starting with ##).
# -----------------------------------------------------------------------------
## help: print targets and short descriptions (default goal)
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'

# -----------------------------------------------------------------------------
# build — Compile the migration tool (no database required).
# -----------------------------------------------------------------------------
## build: compile this module (`go build -o bin/migrate .` into ./bin)
build:
	@mkdir -p bin
	go build -o bin/migrate .

# -----------------------------------------------------------------------------
# migrate — Run the CLI against ScyllaDB (same as `go run .` with flags).
# -----------------------------------------------------------------------------
## migrate: apply CQL migrations (override HOSTS, KEYSPACE, SCRIPTS_DIR)
migrate:
	@echo "Applying migrations: hosts=$(HOSTS) keyspace=$(KEYSPACE) scripts=$(SCRIPTS_DIR)"
	go run . -hosts "$(HOSTS)" -keyspace "$(KEYSPACE)" -scripts-dir "$(SCRIPTS_DIR)"

## test: run migration CLI and script-shape unit tests (no ScyllaDB required)
test:
	go test ./...

# -----------------------------------------------------------------------------
# tidy / fmt / vet — Standard Go maintenance for this module only.
# -----------------------------------------------------------------------------
## tidy: sync go.mod / go.sum for this module
tidy:
	go mod tidy

## fmt: format Go sources in this module
fmt:
	go fmt ./...

## vet: run go vet on this module
vet:
	go vet ./...
