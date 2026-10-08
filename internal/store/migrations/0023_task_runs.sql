-- Each time a task runs: what started it, how it went, and what the agent did. Checks whose
-- output didn't change are recorded too ("unchanged"), so a quiet poll is visible.
CREATE TABLE task_runs (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    agent_id    TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    trigger     TEXT NOT NULL CHECK (trigger IN ('schedule', 'signal', 'manual')),
    outcome     TEXT NOT NULL CHECK (outcome IN ('running', 'quiet', 'acted', 'unchanged', 'failed', 'stopped')),
    detail      TEXT NOT NULL DEFAULT '',
    started_at  INTEGER NOT NULL,
    finished_at INTEGER
);
CREATE INDEX task_runs_task ON task_runs(task_id, started_at);
CREATE INDEX task_runs_started ON task_runs(started_at);
