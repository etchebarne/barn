-- Agents' scheduled tasks. spec: {"cron": "1 10 * * 1-5"} or {"at": <unix ms>}.
CREATE TABLE tasks (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    purpose       TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('cron', 'once')),
    spec          TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    next_fire_at  INTEGER,
    last_fired_at INTEGER,
    created_at    INTEGER NOT NULL
);
CREATE INDEX tasks_due ON tasks(next_fire_at) WHERE enabled = 1;
