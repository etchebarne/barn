-- Actions the user always allows for an agent, without asking ("Always allow" on a card).
-- action is "tool:<name>" for barn's own tools or "connector:<account id>:<tool>" for apps.
-- match is a JSON object of argument values the call must have (e.g. {"channel": "#alerts"});
-- {} allows the action whatever its arguments.
CREATE TABLE approval_rules (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    action     TEXT NOT NULL,
    match      TEXT NOT NULL DEFAULT '{}',
    label      TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (agent_id, action, match)
);
