DROP INDEX IF EXISTS posts_discord_message_id;
ALTER TABLE posts DROP COLUMN discord_message_id;
ALTER TABLE posts DROP COLUMN source;
