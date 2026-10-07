-- Connector accounts (an authenticated account on an external service) and which agents may use
-- them. Credentials are encrypted by the caller.
CREATE TABLE connector_accounts (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    name        TEXT NOT NULL,
    credentials TEXT NOT NULL,
    config      TEXT NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL
);

CREATE TABLE grants (
    agent_id   TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES connector_accounts(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, account_id)
);

-- Incoming events from connectors (kept for debugging and for tasks to inspect).
CREATE TABLE signals (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES connector_accounts(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    payload     TEXT NOT NULL,
    received_at INTEGER NOT NULL
);
CREATE INDEX signals_account ON signals(account_id, id);

-- Signal tasks: kind 'signal' with spec {"accountId", "type", "match": {...}}.
-- SQLite can't alter CHECK constraints, so rebuild tasks to allow it.
CREATE TABLE tasks_new (
    id            TEXT PRIMARY KEY,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    purpose       TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('cron', 'once', 'signal')),
    spec          TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    next_fire_at  INTEGER,
    last_fired_at INTEGER,
    created_at    INTEGER NOT NULL
);
INSERT INTO tasks_new SELECT * FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;
CREATE INDEX tasks_due ON tasks(next_fire_at) WHERE enabled = 1;
