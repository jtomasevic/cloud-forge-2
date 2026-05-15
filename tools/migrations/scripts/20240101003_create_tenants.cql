USE cloudforge;

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY,
    account_id UUID,
    slug TEXT,
    region TEXT,
    status TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tenants_by_account (
    account_id UUID,
    tenant_id UUID,
    slug TEXT,
    status TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY (account_id, created_at, tenant_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS tenants_by_slug (
    slug TEXT PRIMARY KEY,
    tenant_id UUID,
    account_id UUID,
    status TEXT
);
