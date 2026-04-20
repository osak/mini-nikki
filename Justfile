default: run

# 開発サーバー起動（ホットリロード）
dev:
    air

# ビルド
build:
    templ generate
    go build -o ./tmp/main .

# 実行
run:
    templ generate
    go run .

# templ コード生成
generate:
    templ generate
