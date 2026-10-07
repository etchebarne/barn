-- Emoji reactions on messages. reactor is 'user' or 'agent:<id>'.
CREATE TABLE reactions (
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    reactor    TEXT NOT NULL,
    emoji      TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (message_id, reactor, emoji)
);
