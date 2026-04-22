ミニ日記（ゴママヨ）
================

# Local Development
```
cp config.toml.example config.toml

just run
```

# Deployment
初期状態では `caddy_services` というnetworkに接続する。
```
cp config.toml.example config.toml
# admin pass変更

docker compose up -d
```
