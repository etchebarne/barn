-- An agent's personality: how it comes across (tone, voice, manner), kept apart from its
-- instructions (what it does) and its memories (what it knows).
ALTER TABLE agents ADD COLUMN personality TEXT NOT NULL DEFAULT '';
