-- The "your last reply was plain text" nudge is now shown for one model call and never stored.
-- Drop the ones already stored, with the plain-text reply each one answered: a pile of them
-- taught some models never to end a turn with plain text.
DELETE FROM context_entries WHERE id IN (
    SELECT prev_id FROM (
        SELECT entry,
               LAG(id) OVER (PARTITION BY agent_id ORDER BY id) AS prev_id,
               LAG(entry) OVER (PARTITION BY agent_id ORDER BY id) AS prev_entry
        FROM context_entries
    )
    WHERE entry LIKE '%[system] Your last reply was plain text, which nobody can see.%'
      AND json_extract(prev_entry, '$.role') = 'assistant'
      AND json_extract(prev_entry, '$.tool_calls') IS NULL
)
OR entry LIKE '%[system] Your last reply was plain text, which nobody can see.%';
