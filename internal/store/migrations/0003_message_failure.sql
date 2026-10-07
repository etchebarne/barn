-- For system messages reporting a failed agent turn: JSON {"agentId", "reason", "retryable"}.
ALTER TABLE messages ADD COLUMN failure TEXT;
