-- Timestamps are Unix milliseconds (UTC).

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_expires_at ON sessions(expires_at);

CREATE TABLE agents (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    instructions  TEXT NOT NULL,
    model         TEXT NOT NULL,
    language      TEXT NOT NULL DEFAULT 'auto',
    notifications INTEGER NOT NULL DEFAULT 1,
    trust_mode    TEXT NOT NULL DEFAULT 'ask' CHECK (trust_mode IN ('ask', 'trusted')),
    is_admin      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    archived_at   INTEGER
);

CREATE TABLE chats (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('dm', 'group')),
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

-- The user is implicitly a member of every chat.
CREATE TABLE chat_members (
    chat_id  TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (chat_id, agent_id)
);
CREATE INDEX chat_members_agent ON chat_members(agent_id);

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    chat_id         TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    author_kind     TEXT NOT NULL CHECK (author_kind IN ('user', 'agent', 'system')),
    author_agent_id TEXT REFERENCES agents(id),
    body            TEXT NOT NULL,
    created_at      INTEGER NOT NULL
);
CREATE INDEX messages_chat ON messages(chat_id, id);

-- reader is 'user' or 'agent:<id>'.
CREATE TABLE reads (
    chat_id         TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    reader          TEXT NOT NULL,
    last_message_id TEXT NOT NULL,
    PRIMARY KEY (chat_id, reader)
);

-- Agent inbox.
CREATE TABLE events (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    payload     TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    consumed_at INTEGER
);
CREATE INDEX events_pending ON events(agent_id, id) WHERE consumed_at IS NULL;

-- An agent's working context: model-format messages, in order.
CREATE TABLE context_entries (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    entry      TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX context_entries_agent ON context_entries(agent_id, id);
