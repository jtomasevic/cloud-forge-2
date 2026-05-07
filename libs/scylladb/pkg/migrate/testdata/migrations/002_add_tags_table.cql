CREATE TABLE IF NOT EXISTS cf_migrate_test.tags (
    id   text PRIMARY KEY,
    tag  text
);

CREATE INDEX IF NOT EXISTS ON cf_migrate_test.tags (tag)
