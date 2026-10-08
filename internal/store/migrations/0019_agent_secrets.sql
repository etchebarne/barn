-- Secrets the user gave an agent (API tokens and the like), encrypted. Agents get them as
-- environment variables in their computer and never see the values.
CREATE TABLE agent_secrets (
    agent_id    TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    value       TEXT NOT NULL, -- sealed with the server's secret key
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (agent_id, name)
);
