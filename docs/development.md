# 開発ガイド

## 前提ツール

- Go 1.26+
- [templ](https://templ.guide/) CLI（`go install github.com/a-h/templ/cmd/templ@latest`）
- [air](https://github.com/air-verse/air)（ホットリロード）
- [just](https://github.com/casey/just)（タスクランナー）

## セットアップ

```bash
cp config.toml.example config.toml
# config.toml の admin.user / admin.password を設定
```

DB は初回起動時に自動作成・マイグレーション適用される（デフォルト: `mini-nikki.db`）。

## 開発サーバー

```bash
just dev   # air によるホットリロード
# または
just run   # 単発起動
```

air は `.go` / `.templ` / `.css` の変更を検知し、`templ generate && go build` を自動実行する。

## templ コード生成

`.templ` ファイルを編集したら必ずコード生成を実行すること。生成された `*_templ.go` はコミット対象。

```bash
just generate   # templ generate の短縮形
```

## ビルド

```bash
just build   # templ generate + go build -o ./tmp/main .
```

## テスト

```bash
go test ./...
```

モデル層のテストはテンポラリファイルの SQLite DB を使い、実際のマイグレーションを適用してから実行する。

## デプロイ

Docker イメージをビルドして compose で起動する。

```bash
docker compose up -d --build
```

- `config.toml` はコンテナに読み取り専用マウントされる。
- DB データは Docker named volume（`data`）に永続化される。
- Caddy の外部ネットワーク（`caddy_services`）に接続して公開する。

## マイグレーション

`db/migrations/` に連番 SQL ファイルを追加し、`db.Open()` 呼び出し時に自動適用される。

```
NNN_description.up.sql    # 適用
NNN_description.down.sql  # ロールバック
```
