CREATE TABLE likes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    ip_address TEXT    NOT NULL,
    cookie_id  TEXT,
    liked_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE UNIQUE INDEX likes_post_ip_nocookie
    ON likes(post_id, ip_address) WHERE cookie_id IS NULL;
