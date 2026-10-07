-- The user's sidebar: categories (in order, collapsible) and where each chat sits. A chat's
-- position orders it within its section once the user arranged it; NULL (new or never moved)
-- sorts first, by activity.
CREATE TABLE sidebar_categories (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL,
    collapsed  INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
ALTER TABLE chats ADD COLUMN category_id TEXT REFERENCES sidebar_categories(id) ON DELETE SET NULL;
ALTER TABLE chats ADD COLUMN position INTEGER;
