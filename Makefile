.PHONY: all build test lint codegen

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

all: build

build:
	@for m in $(MODULES); do \
		echo "Building $$m..."; \
		(cd $$m && go build ./...) || exit 1; \
	done

test:
	@for m in $(MODULES); do \
		echo "Testing $$m..."; \
		(cd $$m && go test -race -count=1 ./...) || exit 1; \
	done

lint:
	@for m in $(MODULES); do \
		echo "Vetting $$m..."; \
		(cd $$m && go vet ./...) || exit 1; \
	done

codegen:
	@for m in $(MODULES); do \
		(cd $$m && go generate ./... 2>/dev/null || true); \
	done
