-- M0: no domain tables yet. The schema from docs/spec/02-data-model.md
-- arrives with M1. This migration only records when the database was created.
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO meta(key, value) VALUES ('created_at', datetime('now'));
