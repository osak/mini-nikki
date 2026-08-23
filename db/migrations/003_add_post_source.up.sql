ALTER TABLE posts ADD COLUMN source TEXT NOT NULL DEFAULT 'web';
ALTER TABLE posts ADD COLUMN discord_message_id TEXT;

-- Discord メッセージ 1 件につき記事は 1 件まで。
-- 部分インデックスなので web 由来（NULL）の投稿は制約を受けない。
CREATE UNIQUE INDEX posts_discord_message_id
    ON posts(discord_message_id) WHERE discord_message_id IS NOT NULL;
