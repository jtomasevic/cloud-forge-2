USE cloudforge;

CREATE TABLE IF NOT EXISTS networks (
    id UUID PRIMARY KEY,
    tenant_id UUID,
    region TEXT,
    pod_cidr TEXT,
    svc_cidr TEXT,
    status TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS networks_by_tenant (
    tenant_id UUID,
    network_id UUID,
    region TEXT,
    status TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY (tenant_id, created_at, network_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
