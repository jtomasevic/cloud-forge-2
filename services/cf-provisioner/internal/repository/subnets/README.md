# `subnets` repository

This package persists CF-Provisioner logical subnet records in ScyllaDB. These rows are control-plane
metadata, not Kubernetes `Subnet` resources. They let later app-service placement flows validate that a
requested subnet exists, belongs to the requested private network, and is either `private` or `public`.

## Tables

- `subnets`: primary row by subnet `id`.
- `subnets_by_network`: listing index for `GET /v1/networks/{networkId}/subnets`.
- `subnets_by_network_cidr`: lookup index reserved with `IF NOT EXISTS` to reject duplicate canonical
  CIDRs inside one network.

The repository reserves the `(network_id, cidr)` row first with a lightweight transaction, then writes
the primary and listing rows. Scylla does not make those later denormalized writes part of the same
transaction; this matches the existing jobs repository pattern and keeps reads cheap for the current
development control plane.

## Query Patterns

- `Create(ctx, params)` validates UUID shape, rejects duplicate network CIDR rows, and inserts the
  primary plus index records.
- `GetByID(ctx, networkID, subnetID)` loads the primary row and verifies ownership by `networkID`.
- `ListByNetwork(ctx, networkID)` reads the denormalized network partition.

## Service Usage

`internal/service` owns request validation that is closer to API semantics: subnet type, required CIDR,
and canonical IPv4 CIDR parsing. This repository owns persistence concerns and repository-level typed
errors for not-found and duplicate-CIDR cases.
