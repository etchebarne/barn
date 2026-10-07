-- JSON array of agent ids @mentioned in a message (group chats).
ALTER TABLE messages ADD COLUMN mentions TEXT;
