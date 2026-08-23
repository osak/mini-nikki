# 機能仕様

## ルート一覧

| メソッド | パス | 説明 | 認証 |
|---|---|---|---|
| GET | `/` | 投稿一覧（直近 20 件） | 不要 |
| GET | `/posts/{year}/{month}` | 月別アーカイブ | 不要 |
| POST | `/posts/{id}/like` | Like 追加（JSON API） | 不要 |
| GET | `/feed.rss` | RSS 2.0 フィード（直近 20 件） | 不要 |
| GET | `/feed.atom` | Atom 1.0 フィード（直近 20 件） | 不要 |
| GET | `/admin` | 管理画面 | Basic 認証 |
| POST | `/admin/posts` | 投稿作成 | Basic 認証 |
| POST | `/admin/posts/{id}/delete` | 投稿削除 | Basic 認証 |
| POST | `/webhooks/discord` | Discord Interactions エンドポイント | Ed25519 署名検証 |

`/webhooks/discord` は `config.toml` の `discord.public_key` が設定されている場合のみ登録される。

## データモデル

### posts テーブル

```sql
CREATE TABLE posts (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    body               TEXT    NOT NULL,
    created_at         INTEGER NOT NULL DEFAULT (unixepoch()),
    source             TEXT    NOT NULL DEFAULT 'web',  -- 'web' | 'discord'
    discord_message_id TEXT                             -- Discord メッセージ由来の投稿のみ
);

-- 同じ Discord メッセージから記事が二重に作られるのを防ぐ。
-- 部分インデックスなので web 投稿（NULL）は制約を受けない。
CREATE UNIQUE INDEX posts_discord_message_id
    ON posts(discord_message_id) WHERE discord_message_id IS NOT NULL;
```

`created_at` は Unix 秒。JST での日付降順・同日内は時刻昇順で表示する。

`source` は投稿の由来。管理画面からの投稿は `web`、Discord 連携経由は `discord`。
`discord_message_id` はメッセージコマンド由来の投稿にのみ入り、スラッシュコマンド
由来（元メッセージが存在しない）と web 投稿では NULL。

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
    ID               int64
    Body             string
    CreatedAt        time.Time
    LikeCount        int64   // 全ユーザーの合計 Like 数
    HasLiked         bool    // 現在のユーザーが 1 件以上 Like しているか
    Source           string  // model.SourceWeb / model.SourceDiscord
    DiscordMessageID string  // Discord メッセージ由来のみ。それ以外は ""
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
- 投稿フォーム（本文必須）。
- 投稿一覧に各記事のいいね数（♥ N）と編集・削除ボタンを表示。Discord 由来の記事には `Discord` バッジが付く。
- 投稿・削除後は `/admin` へリダイレクト（PRG パターン）。

### バリデーション
- 本文が空 → エラーメッセージ表示。

## Discord 連携

Discord に書いた内容をそのまま日記の記事にする機能。

### 受け口の選択

Discord には「チャンネルの投稿を外部 URL へ push する送信 Webhook」が存在しない
（`https://discord.com/api/webhooks/...` は Discord へ *送る* ための受信 Webhook）。
Discord から HTTP で通知を受け取る公式の手段は **Interactions Endpoint URL** だけなので、
これを受け口としている。Gateway への常時 WebSocket 接続は、常駐プロセスと再接続管理が
増えるため採用していない。

結果として「チャンネルに書いたら自動で記事になる」のではなく、
**実行者が明示的にコマンドを起動したものだけが記事になる**。

### コマンド

| 種別 | 名前 | 記事の本文 | `discord_message_id` |
|---|---|---|---|
| スラッシュコマンド（CHAT_INPUT） | `/nikki body:<本文>` | `body` オプションの値 | NULL |
| メッセージコマンド（MESSAGE） | メッセージ右クリック → 「日記に投稿」 | 対象メッセージの本文 | 対象メッセージ ID |

本文は前後の空白を除去して保存する。既存の投稿と同じく Markdown として描画される。
改行を含む記事はスラッシュコマンドでは入力しづらいので、メッセージコマンドの利用を推奨する。

添付ファイルや埋め込みは取り込まない（Discord の添付 URL は署名付きで期限切れになるため）。

### 認証・認可

1. **署名検証** — `X-Signature-Ed25519` / `X-Signature-Timestamp` ヘッダを
   Developer Portal の Public Key で検証する。署名対象は「タイムスタンプ + 生のリクエストボディ」。
   検証に失敗したら **401** を返す（Discord はエンドポイント登録時に不正な署名を送り、
   401 が返ることを確認する。ここで 401 以外を返すと登録に失敗する）。
2. **ユーザー許可リスト** — `config.toml` の `discord.allowed_user_ids` に含まれる
   Discord ユーザー ID からの実行だけを受け付ける。記事は公開されるため、
   `discord.public_key` を設定する場合 `allowed_user_ids` は必須（空だと起動時にエラー終了）。

### 応答

すべて ephemeral（実行者にだけ見える）メッセージで返す。

| 状況 | 応答 |
|---|---|
| 成功 | `投稿しました（#123）` |
| 許可リスト外のユーザー | `この操作を実行する権限がありません。` |
| 同じメッセージの再投稿 | `このメッセージは既に投稿済みです。` |
| 本文が空 | `本文が空のメッセージは投稿できません。` |
| DB エラー | `投稿に失敗しました。時間をおいて再試行してください。` |

エラー時も HTTP 200 + ephemeral メッセージで返す。HTTP エラーを返すと Discord 上は
「アプリケーションが応答しませんでした」としか表示されず、実行者が原因を知る手立てがなくなるため。

### 管理画面での表示

Discord 由来の記事には管理画面の投稿一覧に `Discord` バッジが付く。編集・削除は web 投稿と同じ。
