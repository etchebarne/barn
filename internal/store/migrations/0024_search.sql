-- Full-text search over message bodies. The index reads from messages (external content) and
-- is kept in step by triggers. It refers to rows by rowid, which VACUUM may renumber: after a
-- VACUUM, rebuild it with INSERT INTO messages_fts(messages_fts) VALUES ('rebuild').
CREATE VIRTUAL TABLE messages_fts USING fts5(
    body,
    content = 'messages',
    content_rowid = 'rowid',
    tokenize = 'unicode61 remove_diacritics 2'
);
INSERT INTO messages_fts(rowid, body) SELECT rowid, body FROM messages;

CREATE TRIGGER messages_fts_insert AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, body) VALUES (new.rowid, new.body);
END;
CREATE TRIGGER messages_fts_delete AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
END;
CREATE TRIGGER messages_fts_update AFTER UPDATE OF body ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, body) VALUES ('delete', old.rowid, old.body);
    INSERT INTO messages_fts(rowid, body) VALUES (new.rowid, new.body);
END;
