-- What each model call cost, as the provider reported it.
CREATE TABLE model_usage (
    id                 TEXT PRIMARY KEY,
    agent_id           TEXT REFERENCES agents(id) ON DELETE CASCADE,
    model              TEXT NOT NULL,
    purpose            TEXT NOT NULL, -- turn | task | compaction | subagent
    prompt_tokens      INTEGER NOT NULL,
    cached_tokens      INTEGER NOT NULL,
    cache_write_tokens INTEGER NOT NULL,
    completion_tokens  INTEGER NOT NULL,
    reasoning_tokens   INTEGER NOT NULL,
    created_at         INTEGER NOT NULL
);
CREATE INDEX model_usage_agent ON model_usage(agent_id, created_at);
CREATE INDEX model_usage_time ON model_usage(created_at);
