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

## Discord 連携のセットアップ

`config.toml` の `discord.public_key` が空の場合、連携は無効（`/webhooks/discord` は登録されない）。

### 1. Discord アプリを作る

[Developer Portal](https://discord.com/developers/applications) で New Application。

### 2. config.toml を設定する

```toml
[discord]
# General Information > Public Key
public_key = "xxxxxxxx..."
# 投稿を許可するユーザー ID（設定 > 詳細設定 > 開発者モード を ON にして
# 自分のアイコンを右クリック > 「ユーザー ID をコピー」）
allowed_user_ids = ["123456789012345678"]
```

記事は公開されるため `allowed_user_ids` は必須。空のままサーバーを起動するとエラー終了する。

### 3. コマンドを登録する

`APP_ID`（General Information > Application ID）と `BOT_TOKEN`（Bot > Reset Token）を用意して実行する。

スラッシュコマンド `/nikki`：

```bash
curl -X POST "https://discord.com/api/v10/applications/$APP_ID/commands" -H "Authorization: Bot $BOT_TOKEN" -H "Content-Type: application/json" -d '{"name":"nikki","type":1,"description":"ミニ日記に投稿する","options":[{"name":"body","type":3,"description":"本文（Markdown 可）","required":true}]}'
```

メッセージコマンド「日記に投稿」（メッセージ右クリックから起動）：

```bash
curl -X POST "https://discord.com/api/v10/applications/$APP_ID/commands" -H "Authorization: Bot $BOT_TOKEN" -H "Content-Type: application/json" -d '{"name":"日記に投稿","type":3}'
```

`type: 1`（CHAT_INPUT）の `name` は小文字英数字のみ。`type: 3`（MESSAGE）に `description` は付けられない。
グローバルコマンドの反映には最大 1 時間かかる。すぐ試すなら URL を
`/applications/$APP_ID/guilds/$GUILD_ID/commands` に変えてギルドコマンドとして登録する。

### 4. エンドポイント URL を登録する

サーバーを公開した状態で、General Information > **Interactions Endpoint URL** に
`https://<ホスト名>/webhooks/discord` を設定して保存する。

Discord は保存時に正しい署名と不正な署名の両方でリクエストを送り、前者に PONG、後者に 401 が
返ることを確認する。保存が通ればエンドポイントは正しく動いている。

ローカル開発では公開 URL が必要になるので、`cloudflared tunnel --url http://localhost:8080` などで
トンネルを張り、その URL を登録する。

### 5. アプリをサーバーに導入する

Installation（または OAuth2 > URL Generator）で `applications.commands` スコープを含む
インストール URL を生成し、使うサーバー / ユーザーに追加する。

### 受信ペイロードを確認する

署名検証を通過した Interaction は、生の JSON がそのままログに出る。

```
INFO discord: received interaction payload="{\"id\":\"2\",\"type\":2,...}"
```

**署名検証に失敗したリクエストのボディは記録しない。** Discord 由来である保証がなく、
ログ汚染や肥大化の的になるため、メタデータのみ残す。

```
WARN discord: rejected request with invalid signature remote_addr=::1 body_bytes=56 has_signature=true has_timestamp=true
```

整形して読むには：

```bash
docker compose logs mini-nikki | grep 'received interaction' | sed 's/.*payload=//' | python3 -c 'import sys,json;[print(json.dumps(json.loads(json.loads(l)),ensure_ascii=False,indent=2)) for l in sys.stdin]'
```

ペイロードには Interaction の `token`（フォローアップメッセージ送信に使える、有効期限 15 分の
credential）が含まれる点に注意。ログの取り扱いが気になる場合はこのフィールドをマスクする。

8KB を超えるペイロードは切り詰められ、末尾に `...(truncated, N bytes total)` が付く。

### トラブルシューティング

| 症状 | 原因 |
|---|---|
| Endpoint URL の保存に失敗する | 署名検証が通っていない。`public_key` の値、リバースプロキシがボディを書き換えていないかを確認 |
| 「アプリケーションが応答しませんでした」 | 3 秒以内に応答できていない。サーバーログを確認 |
| `この操作を実行する権限がありません。` | 実行者の ID が `allowed_user_ids` にない |
| コマンドは通るのに記事にならない | `discord: received interaction` のペイロードと、直後の `WARN` 行を確認 |

## マイグレーション

`db/migrations/` に連番 SQL ファイルを追加し、`db.Open()` 呼び出し時に自動適用される。

```
NNN_description.up.sql    # 適用
NNN_description.down.sql  # ロールバック
```
