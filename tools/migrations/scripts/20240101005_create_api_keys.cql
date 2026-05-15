USE cloudforge;

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY,
    account_id UUID,
    key_hash TEXT,
    key_prefix TEXT,
    created_at TIMESTAMP,
    revoked_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_keys_by_account (
    account_id UUID,
    key_id UUID,
    key_prefix TEXT,
    created_at TIMESTAMP,
    revoked_at TIMESTAMP,
    PRIMARY KEY (account_id, created_at, key_id)
) WITH CLUSTERING ORDER BY (created_at DESC);

CREATE TABLE IF NOT EXISTS api_keys_by_hash (
    key_hash TEXT PRIMARY KEY,
    key_id UUID,
    account_id UUID,
    revoked_at TIMESTAMP
);
