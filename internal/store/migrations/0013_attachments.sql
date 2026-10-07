-- Files and images attached to messages. An upload belongs to its chat until it's sent with a
-- message (message_id set); unsent uploads are cleaned up. The bytes live on disk under
-- <data>/shared/attachments/<id>/<name>.
CREATE TABLE attachments (
    id         TEXT PRIMARY KEY,
    chat_id    TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id TEXT REFERENCES messages(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL DEFAULT 0,
    name       TEXT NOT NULL,
    mime       TEXT NOT NULL,
    size       INTEGER NOT NULL,
    width      INTEGER,
    height     INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX attachments_message ON attachments(message_id, position);
