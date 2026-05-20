-- Enable pgvector. Used in Phase 3 for long-term memory / RAG.
CREATE EXTENSION IF NOT EXISTS vector;

-- Phase 1 placeholder. Real schema is created in later phases by migrations.
CREATE TABLE IF NOT EXISTS server_info (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO server_info (key, value)
VALUES ('initialized_at', NOW()::TEXT)
ON CONFLICT (key) DO NOTHING;
