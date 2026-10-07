-- Durable facts an agent chose to remember (private to that agent).
CREATE TABLE memories (
    id             TEXT PRIMARY KEY,
    agent_id       TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    text           TEXT NOT NULL,
    source_chat_id TEXT REFERENCES chats(id) ON DELETE SET NULL,
    created_at     INTEGER NOT NULL
);
CREATE INDEX memories_agent ON memories(agent_id, id);

-- One rolling summary per agent of the context compacted away so far.
CREATE TABLE context_summaries (
    agent_id   TEXT PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    content    TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
