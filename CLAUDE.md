# ミニブログ実装指示書

Go + templ を使ったSSRのシンプルなミニブログWebアプリを実装する。

名前は「ミニ日記（ゴママヨ）」とする。

---

## 技術スタック

- **言語**: Go 1.22+
- **テンプレート**: [templ](https://templ.guide/)
- **ルーター**: `net/http` 標準ライブラリ（`http.ServeMux`）
- **DB**: SQLite（`modernc.org/sqlite` — CGO不要）
- **マイグレーション**: `golang-migrate/migrate`
- **静的ファイル**: `embed.FS` でバイナリ埋め込み
- **ホットリロード**: `air`（開発時のみ）
- **タスクランナー**: Just

---

## ディレクトリ構成

```
.
├── CLAUDE.md
├── go.mod
├── go.sum
├── .air.toml
├── main.go
├── db/
│   ├── db.go              # DB接続・初期化
│   └── migrations/
│       ├── 001_create_posts.up.sql
│       └── 001_create_posts.down.sql
├── handler/
│   ├── post.go            # 投稿一覧・詳細・作成・削除
│   └── middleware.go      # ログ等のミドルウェア
├── model/
│   └── post.go            # Postの構造体・DBアクセス関数
├── templates/
│   ├── layout.templ       # 共通レイアウト
│   ├── index.templ        # 投稿一覧ページ
│   ├── post.templ         # 投稿詳細ページ
│   └── components/
│       └── post_card.templ
└── static/
    └── style.css
```

---

## データモデル

```sql
-- db/migrations/001_create_posts.up.sql
CREATE TABLE posts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    body       TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

```go
// model/post.go
type Post struct {
    ID        int64
    Body      string
    CreatedAt time.Time
}
```

---

## 機能要件

### ページ一覧

| ルート | メソッド | 説明 |
|---|---|---|
| `GET /` | GET | 投稿一覧（新しい順、20件） |
| `GET /posts/{id}` | GET | 投稿詳細 |
| `POST /admin/posts` | POST | 投稿作成（フォームsubmit） |
| `POST /admin/posts/{id}/delete` | POST | 投稿削除（HTMLフォームのためPOSTで代替） |

### 投稿一覧（`/`）

- 投稿フォーム（テキストエリア + 投稿ボタン）を上部に表示
- 投稿一覧をカード形式で表示
- 各カードに投稿日時と削除ボタンを表示
- 投稿後は `http.Redirect` で `/` にリダイレクト（PRGパターン）

### バリデーション

- 本文が空の場合は投稿を拒否し、エラーメッセージを表示
- 本文は最大280文字

---

## templの使い方

```go
// templates/layout.templ
package templates

templ Layout(title string) {
    <!DOCTYPE html>
    <html lang="ja">
    <head>
        <meta charset="UTF-8"/>
        <title>{ title }</title>
        <link rel="stylesheet" href="/static/style.css"/>
    </head>
    <body>
        { children... }
    </body>
    </html>
}

// templates/index.templ
package templates

import "github.com/yourname/miniblog/model"

templ IndexPage(posts []model.Post, errMsg string) {
    @Layout("ミニブログ") {
        <main>
            @PostForm(errMsg)
            @PostList(posts)
        </main>
    }
}
```

ハンドラでのレンダリング:

```go
func (h *PostHandler) Index(w http.ResponseWriter, r *http.Request) {
    posts, err := h.model.List(r.Context())
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
    templates.IndexPage(posts, "").Render(r.Context(), w)
}
```

---

## 静的ファイルの埋め込み

```go
// main.go
//go:embed static
var staticFS embed.FS

mux.Handle("/static/", http.FileServerFS(staticFS))
```

---

## コード生成ワークフロー

templファイルを編集したら必ずコード生成を実行すること:

```bash
templ generate
```

air を使う場合は `.air.toml` で `templ generate` をpre-buildコマンドとして設定する:

```toml
[build]
  pre_cmd = ["templ generate"]
  cmd = "go build -o ./tmp/main ."
  bin = "./tmp/main"
  include_ext = ["go", "templ"]
```

---

## 実装の進め方

以下の順序で実装すること:

1. `go mod init` と依存パッケージの追加
2. DB初期化・マイグレーション（`db/`）
3. モデル層（`model/post.go`）
4. templテンプレート（`templates/`）
5. `templ generate` でコード生成
6. ハンドラ（`handler/post.go`）
7. `main.go` でルーティング・サーバー起動
8. `static/style.css` でスタイリング

---

## 最初には実装しないもの（スコープ外）

- 認証・ログイン機能
- 画像アップロード
- タグ・カテゴリ
- ページネーション（最初は最新20件固定でよい）
- テスト（初期実装では省略）
