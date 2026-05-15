USE cloudforge;

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY,
    email TEXT,
    status TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS accounts_by_email (
    email TEXT PRIMARY KEY,
    account_id UUID,
    status TEXT
);
