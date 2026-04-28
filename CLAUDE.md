# ミニ日記（ゴママヨ）

Go + templ による SSR のシンプルな日記 Web アプリ。

詳細ドキュメントは `docs/` を参照：
- [`docs/architecture.md`](docs/architecture.md) — 技術スタック・ディレクトリ構成・設計決定
- [`docs/spec.md`](docs/spec.md) — 機能仕様・データモデル・レートリミット
- [`docs/development.md`](docs/development.md) — 開発・ビルド・デプロイ手順

## AI コーディング時の注意

- `.templ` ファイルを変更したら必ず `templ generate`（または `just generate`）を実行すること。生成された `*_templ.go` もコミット対象。
- DB スキーマ変更は `db/migrations/` に連番 SQL ファイルを追加し、アプリ側のコードも合わせて変更すること。
- `model/like.go` の `todayUnix()` は `model/post.go` で定義された `jst` 変数を参照している。
