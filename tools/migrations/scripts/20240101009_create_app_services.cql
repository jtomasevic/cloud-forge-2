USE cloudforge;

-- Primary CF App Service record.
--
-- This row is the durable source needed to reconstruct a tenant workload after a
-- CF-Provisioner restart. Scalar columns keep common reconciliation decisions queryable
-- without parsing JSON. The *_json columns intentionally preserve OpenAPI-shaped nested
-- inputs for the MVP; REST/service validation owns their structure, and the repository
-- layer added in the next task will serialize only validated DTOs into these fields.
CREATE TABLE IF NOT EXISTS app_services (
    id UUID PRIMARY KEY,
    tenant_id UUID,
    network_id UUID,
    subnet_id UUID,
    name TEXT,
    status TEXT,
    service_type TEXT,
    image TEXT,
    build_context TEXT,
    dockerfile TEXT,
    command_json TEXT,
    args_json TEXT,
    cpu TEXT,
    memory TEXT,
    replicas INT,
    env_json TEXT,
    ports_json TEXT,
    exposure_type TEXT,
    exposure_status TEXT,
    exposure_json TEXT,
    swagger_json TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Network-scoped listing index.
--
-- Scylla does not join the primary app_services table when listing a private
-- network's workloads, so this denormalized row carries the summary fields needed by
-- GET /v1/networks/{networkId}/app-services. The primary row remains authoritative
-- for full runtime, exposure, and Swagger/OpenAPI metadata.
CREATE TABLE IF NOT EXISTS app_services_by_network (
    network_id UUID,
    created_at TIMESTAMP,
    app_service_id UUID,
    tenant_id UUID,
    name TEXT,
    subnet_id UUID,
    status TEXT,
    service_type TEXT,
    exposure_type TEXT,
    exposure_status TEXT,
    public_host TEXT,
    updated_at TIMESTAMP,
    PRIMARY KEY (network_id, created_at, app_service_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

-- Public exposure lookup.
--
-- Internet Gateway exposure must be explicit and auditable. This host-index row gives
-- future routing code a single place to resolve a public host to the owning tenant,
-- network, and app service, while keeping Swagger metadata close to the public route
-- that requires it.
CREATE TABLE IF NOT EXISTS app_service_exposures_by_host (
    host TEXT PRIMARY KEY,
    app_service_id UUID,
    tenant_id UUID,
    network_id UUID,
    port_name TEXT,
    tls_enabled BOOLEAN,
    exposure_status TEXT,
    swagger_json TEXT,
    updated_at TIMESTAMP
);

-- App-service lifecycle job index.
--
-- provisioning_jobs remains the generic polling table keyed by job id and network.
-- App-service workflows also need correlation by app_service_id, so this index records
-- the relationship without changing the older network provisioning job queries.
CREATE TABLE IF NOT EXISTS app_service_jobs_by_app_service (
    network_id UUID,
    app_service_id UUID,
    created_at TIMESTAMP,
    job_id UUID,
    tenant_id UUID,
    job_type TEXT,
    status TEXT,
    PRIMARY KEY ((network_id, app_service_id), created_at, job_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
