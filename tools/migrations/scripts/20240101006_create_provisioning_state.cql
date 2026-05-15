USE cloudforge;

CREATE TABLE IF NOT EXISTS provisioning_jobs (
    id UUID PRIMARY KEY,
    tenant_id UUID,
    network_id UUID,
    job_type TEXT,
    status TEXT,
    error_message TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS provisioning_jobs_by_network (
    network_id UUID,
    job_id UUID,
    job_type TEXT,
    status TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY (network_id, created_at, job_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
