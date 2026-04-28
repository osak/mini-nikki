# 機能仕様

## ルート一覧

| メソッド | パス | 説明 | 認証 |
|---|---|---|---|
| GET | `/` | 投稿一覧（直近 20 件） | 不要 |
| GET | `/posts/{year}/{month}` | 月別アーカイブ | 不要 |
| POST | `/posts/{id}/like` | Like 追加（JSON API） | 不要 |
| GET | `/admin` | 管理画面 | Basic 認証 |
| POST | `/admin/posts` | 投稿作成 | Basic 認証 |
| POST | `/admin/posts/{id}/delete` | 投稿削除 | Basic 認証 |

## データモデル

### posts テーブル

```sql
CREATE TABLE posts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    body       TEXT    NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
```

`created_at` は Unix 秒。JST での日付降順・同日内は時刻昇順で表示する。

### likes テーブル

```sql
CREATE TABLE likes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    ip_address TEXT    NOT NULL,
    cookie_id  TEXT,                -- NULL = Cookie なしアクセス
    liked_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Cookie なしの場合、同一 (post, IP) への重複 Like を防ぐ
CREATE UNIQUE INDEX likes_post_ip_nocookie
    ON likes(post_id, ip_address) WHERE cookie_id IS NULL;
```

### Go 構造体

```go
type Post struct {
    ID        int64
    Body      string
    CreatedAt time.Time
    LikeCount int64   // 全ユーザーの合計 Like 数
    HasLiked  bool    // 現在のユーザーが 1 件以上 Like しているか
}
```

## 機能詳細

### 投稿一覧（`/`）
- 直近 20 件を JST 日付降順・同日内時刻昇順で取得し、日付ごとにグループ表示。
- 各投稿カードに Like ボタン・パーマリンクコピーボタンを表示。

### 月別アーカイブ（`/posts/{year}/{month}`）
- 指定月の全投稿を同順で表示。

### Like ボタン
- 匿名で押せる。押すたびに `POST /posts/{id}/like` を fetch で非同期送信。
- サーバーは `{"count": N, "liked": true}` を返す（429 時は `{"count": N, "liked": false, "reason": "rate_limited"}`）。
- ボタンは `♡ N` → `♥ N` へ切り替わる。制限時は一時的に「制限中…」と表示。

#### レートリミット仕様

* 通常の閲覧者に対しては各記事について10件までLikeを付けられるようにする。
* 見えたURLへ無差別にアクセスする質の低いbotがLike数を過度に増やすことを防ぐため、Cookieを送信していることをもって通常ユーザーを判別し、それ以外のアクセスに対してはIPアドレスで厳しい制限を掛ける。
* 悪意をもったユーザーがCookieを消去して繰り返しLikeしたり、Cookie対応だけはできるが既読管理のできないような低品質なbotが繰り返しアクセスしたりといった荒らし行為を防ぐため、同一IPに紐付くCookie数にも制限をかける。

**Cookie なし（`cookie_id IS NULL`）**

| 条件 | 制限 |
|---|---|
| 同一 IP + 同一記事 | 1 件のみ（UNIQUE インデックスで強制） |
| 同一 IP の 1 日合計 | 10 件まで |

**Cookie あり（`nikki_sid` Cookie が存在）**

| 条件 | 制限 |
|---|---|
| 同一 Cookie + 同一記事 | 10 件まで |
| 同一 IP から異なる Cookie が Like できる数（1 日あたり） | 5 種類まで |
| 同一 IP の 1 日合計 Like 数 | 制限なし |

Cookie は初訪問時にサーバーが自動発行（`nikki_sid`、有効期限 1 年）。

### 管理画面（`/admin`）
- Basic 認証（`config.toml` の `admin.user` / `admin.password`）。
- 投稿フォーム（本文必須、最大 280 文字）。
- 投稿一覧に各記事のいいね数（♥ N）と削除ボタンを表示。
- 投稿・削除後は `/admin` へリダイレクト（PRG パターン）。

### バリデーション
- 本文が空 → エラーメッセージ表示。
- 本文が 281 文字以上 → エラーメッセージ表示。
