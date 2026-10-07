-- A message can reply to an earlier message in the same chat.
ALTER TABLE messages ADD COLUMN reply_to TEXT;
