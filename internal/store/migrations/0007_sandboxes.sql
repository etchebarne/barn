-- Agents' sandboxes (Docker containers). Several agents may share one.
CREATE TABLE sandboxes (
    id         TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL
);

ALTER TABLE agents ADD COLUMN sandbox_id TEXT REFERENCES sandboxes(id);
