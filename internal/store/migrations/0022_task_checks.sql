-- A task's check: a command run on schedule in the agent's computer that wakes the agent only
-- when its output changes (the command itself lives in spec). This is its last output.
ALTER TABLE tasks ADD COLUMN check_output TEXT;
