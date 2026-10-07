-- Archiving is gone (agents are deleted instead). Agents archived before come back, so they can
-- be deleted or kept.
ALTER TABLE agents DROP COLUMN archived_at;
