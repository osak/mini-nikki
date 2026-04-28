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
| Markdown | `yuin/goldmark`（ハードラップ有効） |
| 設定ファイル | `BurntSushi/toml` |
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
│       └── 002_create_likes.{up,down}.sql
├── handler/
│   ├── post.go                      # 投稿一覧・月別・管理・作成・削除
│   ├── like.go                      # Like API
│   └── middleware.go                # Logger / BasicAuth / SessionCookie / ClientIP
├── model/
│   ├── post.go                      # Post 構造体・DB アクセス
│   └── like.go                      # LikeModel・レートリミットロジック
├── internal/
│   └── markdown/markdown.go         # goldmark ラッパー
├── templates/
│   ├── layout.templ                 # 共通レイアウト
│   ├── index.templ                  # 投稿一覧ページ
│   ├── month.templ                  # 月別アーカイブページ
│   ├── admin.templ                  # 管理画面
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

### タイムゾーン
`time.FixedZone("JST", 9*60*60)` による UTC+9 固定。`time.LoadLocation` を使わないことで `tzdata` への依存をなくしている。DB には Unix 秒（INTEGER）で保存し、読み出し時に JST へ変換する。

### SQLite ドライバ
`modernc.org/sqlite`（Pure Go）を使用。CGO 不要なのでクロスコンパイルが容易で、`scratch` ベースの Docker イメージにそのまま収まる。

### IP アドレスの取得
本番環境では Caddy がリバースプロキシとなるため、`X-Forwarded-For` ヘッダを優先して参照する（`handler.ClientIP`）。

### セッション Cookie
初訪問時に `SessionCookie` ミドルウェアが `nikki_sid` Cookie（16 バイト乱数の hex 文字列、有効期限 1 年、HttpOnly, SameSite=Lax）を発行し、リクエストコンテキストに格納する。
