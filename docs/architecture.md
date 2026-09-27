# アーキテクチャ

## 概要

「ミニ日記（ゴママヨ）」は Go + templ による SSR のシンプルな日記 Web アプリ。

## 技術スタック

| 役割 | ライブラリ / ツール |
|---|---|
| 言語 | Go 1.26+ |
| テンプレート | [templ](https://templ.guide/) v0.3+ |
| ルーター | `net/http` 標準ライブラリ（`http.ServeMux`） |
| DB | SQLite — `modernc.org/sqlite`（CGO 不要） |
| マイグレーション | `golang-migrate/migrate` v4 |
| Markdown | `yuin/goldmark`（ハードラップ・Linkify 有効） |
| 設定ファイル | `BurntSushi/toml` |
| Discord 署名検証 | `crypto/ed25519`（標準ライブラリ） |
| 静的ファイル | `embed.FS` でバイナリ埋め込み |
| ホットリロード | `air`（開発時のみ） |
| タスクランナー | Just |
| リバースプロキシ | Caddy（本番） |

## ディレクトリ構成

```
.
├── main.go                          # エントリポイント・ルーティング
├── config.toml                      # 管理者認証情報・DB パス
├── Justfile                         # タスク定義
├── Dockerfile / compose.yml         # コンテナ構成
├── db/
│   ├── db.go                        # DB 接続・マイグレーション実行
│   └── migrations/
│       ├── 001_create_posts.{up,down}.sql
│       ├── 002_create_likes.{up,down}.sql
│       ├── 003_add_post_source.{up,down}.sql
│       └── 004_create_link_previews.{up,down}.sql
├── handler/
│   ├── post.go                      # 投稿一覧・月別・管理・作成・編集・削除
│   ├── like.go                      # Like API
│   ├── feed.go                      # RSS / Atom フィード
│   ├── discord.go                   # Discord Interactions エンドポイント
│   ├── link_preview.go              # リンクカードの OGP 読み出し・取得依頼
│   └── middleware.go                # Logger / BasicAuth / SessionCookie / ClientIP
├── model/
│   ├── post.go                      # Post 構造体・DB アクセス
│   ├── like.go                      # LikeModel・レートリミットロジック
│   └── link_preview.go              # LinkPreviewModel（OGP キャッシュ）
├── internal/
│   ├── markdown/
│   │   ├── markdown.go              # goldmark ラッパー
│   │   ├── linkify.go               # 和文に続く URL のリンク化
│   │   ├── embed.go                 # @[card] / @[embed] 記法
│   │   ├── embed.templ              # カード・埋め込みのマークアップ
│   │   └── providers.go             # 埋め込み対応サイト（YouTube / X）
│   └── linkpreview/
│       ├── fetch.go                 # OGP 取得
│       └── worker.go                # バックグラウンド取得ワーカー
├── templates/
│   ├── layout.templ                 # 共通レイアウト
│   ├── index.templ                  # 投稿一覧ページ
│   ├── month.templ                  # 月別アーカイブページ
│   ├── admin.templ                  # 管理画面
│   ├── edit.templ                   # 投稿編集ページ
│   ├── helpers.go                   # テンプレートヘルパー関数
│   └── components/
│       ├── post_card.templ          # 投稿カード
│       └── post_group.templ         # 日付グループ
└── static/
    ├── style.css
    └── app.js
```

`*_templ.go` ファイルは `templ generate` で自動生成される。

## 設計上の決定事項

### DBとしてSQLiteを採用
PostgresやMySQLのサーバーを立てると別途管理する手間がかかり、DBプロセス分のメモリも必要になる。また、小規模なブログサービスであり、記事の投稿がadminに限られているという状況を鑑みると高度なトランザクション管理は必要ないと考えられる。これらの理由からSQLiteを採用した。

データ量が多くなり、読み込み性能が間に合わなくなってきた場合にはSSR結果のキャッシュやCDN化によって負荷対策をする。コメント機能やLike機能によって書き込み性能が間に合わなくなった場合はCQRSのようなモデルに行こうし、書き込みキューの分離を試みる。どちらの場合でも負荷が過度に高まるようならPostgresへの移行を検討する。

### タイムゾーン
`time.FixedZone("JST", 9*60*60)` による UTC+9 固定。`time.LoadLocation` を使わないことで `tzdata` への依存をなくしている。

SQLiteには日付型が存在せず、タイムゾーンの管理もできないため、DBにはタイムスタンプをUnix 秒（INTEGER）で保存する。読み出し時に必要に応じて JST へ変換する。

### SQLite ドライバ
`modernc.org/sqlite`（Pure Go）を使用。CGO 不要なのでクロスコンパイルが容易で、`scratch` ベースの Docker イメージにそのまま収まる。

### IP アドレスの取得
本番環境では Caddy がリバースプロキシとなるため、`X-Forwarded-For` ヘッダを優先して参照する（`handler.ClientIP`）。

### Discord 連携は Interactions Endpoint で受ける
Discord には「チャンネルの投稿を外部 URL へ push する送信 Webhook」が存在しない。Discord から HTTP でイベントを受け取る公式の手段は Interactions Endpoint URL のみであり、これを `POST /webhooks/discord` として実装した。

Gateway（WebSocket）への常時接続なら「チャンネルに書くだけで記事になる」体験が作れるが、常駐接続と再接続・レート制御の管理が必要になり、SSR の HTTP サーバー 1 プロセスという構成が崩れる。日記の投稿頻度を考えると、実行者が明示的にコマンドを起動する Interactions のほうが釣り合いが取れていると判断した。

署名検証は標準ライブラリの `crypto/ed25519` で行うため、Discord 連携のための外部依存は増えていない。

### Discord 由来の投稿の識別
`posts.source`（`web` / `discord`）と `posts.discord_message_id` で由来を保持する。`discord_message_id` の部分 UNIQUE インデックスがメッセージコマンドの二度押しや再送に対する冪等性を担保する。web 投稿では NULL になり、SQLite の UNIQUE インデックスは NULL を重複とみなさないため制約に触れない。

UNIQUE 制約違反の判定（`model.isUniqueViolation`）はエラーメッセージの文字列一致で行っている。`modernc.org/sqlite` がドライバ固有のエラー型を公開していないため。

### 和文に続く URL のリンク化
goldmark の Linkify 拡張は URL の直前が空白か `*_~(` のときにしか発火しない。さらに goldmark はインラインパーサを行頭・空白・ASCII 記号の位置でしか呼び出さないため、トリガー文字を増やしても和文の直後では呼ばれない。そこで `internal/markdown/linkify.go` で AST Transformer を追加し、パース後のテキストノードから「非 ASCII 文字の直後の URL」を探して AutoLink に置き換えている。URL の終端判定は Linkify のパーサに委譲し、挙動を本家と揃えている。

### リンクカードの OGP は保存時に取得してキャッシュする
表示時に外部サイトへ取りに行くと、ページの表示速度が外部サイトの応答に律速される。また Discord Interactions は 3 秒以内に応答する必要があるため、投稿処理の中でも取得できない。そこで取得は `linkpreview.Worker`（goroutine 1 本）が行い、結果を `link_previews` テーブルにキャッシュする。ハンドラはキャッシュを読むことと、取得依頼をキューに積むことしかしない。キャッシュが無いカードは URL だけのフォールバック表示にする。

取得依頼は投稿の作成・編集時に加えて、表示時にキャッシュが無い URL についても積む。これで既存投稿のバックフィルや失敗した URL の再試行のための仕組みを別に持たずに済む。キューが溢れた依頼は捨てるが、次の表示時に積み直される。

OGP の取得先はサーバーから内部ネットワークを叩けないよう、接続先がループバック・プライベートアドレスなら拒否する（投稿者は管理者だけだが、念のため）。

### 埋め込みは許可リスト方式で自前生成する
oEmbed の `html` をそのまま差し込むと、外部サイト由来の HTML がページに入り XSS の余地が生まれる。投稿者が管理者だけでも HTML の出どころは外部サイトなので、対応サイトを許可リストで持ち、URL から ID を取り出してマークアップを自前で組み立てる。対応していないサイトはリンクカードにフォールバックする。

### セッション Cookie
初訪問時に `SessionCookie` ミドルウェアが `nikki_sid` Cookie（16 バイト乱数の hex 文字列、有効期限 1 年、HttpOnly, SameSite=Lax）を発行し、リクエストコンテキストに格納する。
