-- リンクカード（@[card](url) / @[embed](url)）用の OGP キャッシュ。
-- 投稿ではなく URL をキーにして、同じ URL を複数の投稿で共有する。
CREATE TABLE link_previews (
    url          TEXT    PRIMARY KEY,
    status       TEXT    NOT NULL,          -- 'ok' | 'failed'
    title        TEXT    NOT NULL DEFAULT '',
    description  TEXT    NOT NULL DEFAULT '',
    image_url    TEXT    NOT NULL DEFAULT '',
    site_name    TEXT    NOT NULL DEFAULT '',
    fetched_at   INTEGER NOT NULL
);
