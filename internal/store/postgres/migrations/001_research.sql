-- Phase 3: research session persistence.

CREATE TABLE IF NOT EXISTS research_sessions (
    id            UUID PRIMARY KEY,
    question      TEXT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    plan          JSONB,
    report        TEXT NOT NULL DEFAULT '',
    elapsed_ms    BIGINT,
    char_count    INT,
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_research_sessions_created_at
    ON research_sessions (created_at DESC);

CREATE TABLE IF NOT EXISTS research_tasks (
    session_id    UUID NOT NULL REFERENCES research_sessions (id) ON DELETE CASCADE,
    task_id       TEXT NOT NULL,
    question      TEXT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('pending', 'running', 'done', 'failed')),
    findings      TEXT NOT NULL DEFAULT '',
    citations     JSONB NOT NULL DEFAULT '[]'::jsonb,
    elapsed_ms    BIGINT,
    error_message TEXT,
    sort_order    INT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_research_tasks_session
    ON research_tasks (session_id, sort_order);
