-- prompt: JSON for agent messages that ask the user something (see store.Prompt).
-- event: JSON for system messages that mark something that happened (see store.MessageEvent).
ALTER TABLE messages ADD COLUMN prompt TEXT;
ALTER TABLE messages ADD COLUMN event TEXT;
