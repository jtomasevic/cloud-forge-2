USE cloudforge;

CREATE TABLE IF NOT EXISTS subnets (
    id UUID PRIMARY KEY,
    network_id UUID,
    type TEXT,
    cidr TEXT,
    zone TEXT,
    created_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subnets_by_network (
    network_id UUID,
    subnet_id UUID,
    type TEXT,
    cidr TEXT,
    zone TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY (network_id, subnet_id)
);

CREATE TABLE IF NOT EXISTS subnets_by_network_cidr (
    network_id UUID,
    cidr TEXT,
    subnet_id UUID,
    type TEXT,
    zone TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY (network_id, cidr)
);
