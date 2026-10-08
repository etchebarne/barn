-- Each agent's system prompt, frozen between rebuilds so providers can cache it (with everything
-- after it). fingerprint identifies what it was built from; memories aren't part of it, so an
-- agent saving a memory doesn't rebuild it.
ALTER TABLE agents ADD COLUMN prompt_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN prompt_fingerprint TEXT NOT NULL DEFAULT '';
