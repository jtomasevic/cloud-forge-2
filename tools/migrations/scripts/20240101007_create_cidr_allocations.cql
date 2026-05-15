USE cloudforge;

CREATE TABLE IF NOT EXISTS cidr_allocations (
    network_id UUID PRIMARY KEY,
    pod_cidr TEXT,
    svc_cidr TEXT,
    allocated_at TIMESTAMP
);
