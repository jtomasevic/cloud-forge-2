# CloudForge

An open-source multi-tenant cloud platform built on Kubernetes.

## Repository structure

- `api/` — OpenAPI 3.0.3 specifications (source of truth for HTTP contracts)
- `services/` — CloudForge platform services (cf-router, cf-accounts, cf-provisioner)
- `libs/` — Shared libraries and generated client SDKs
- `tools/` — CLI tools and schema migration runner
- `dev/` — Local development environment configuration

## Prerequisites

Go 1.26+, Docker, k3d, kubectl, Helm, Tilt, oapi-codegen v2.

See `docs/plan/README.md` for the full development setup guide.

## Quick start

- See `docs/plan/README.md`.
- Check other documents under `docs/`

