# FoundationDB Dashboard

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Русский](README.ru.md) | **日本語** | [Deutsch](README.de.md) | [Français](README.fr.md)

FoundationDB クラスター向けの非公式 Docker Web コンソールです。1 つのイメージに UI と FoundationDB **7.3.62** クライアントが含まれます。データベース本体はイメージの外にあります。

FoundationDB: [github.com/apple/foundationdb](https://github.com/apple/foundationdb)  
イメージ: [hub.docker.com/r/runu/foundationdb-dashboard](https://hub.docker.com/r/runu/foundationdb-dashboard)

## クイックスタート

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  runu/foundationdb-dashboard:7.3.62
```

すでに Compose でクラスターが動いている場合:

```bash
docker compose up -d
```

[http://127.0.0.1:8090/](http://127.0.0.1:8090/) を開きます。

ポートは `127.0.0.1` にだけ公開してください。プロセスは `PORT`（既定 `8080`）を待ち受けます。

## 設定

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `FDB_CONNECTION_STRING` | いずれか | 接続文字列。例: `docker:docker@foundationdb:4500` |
| `FDB_CLUSTER_FILE` | いずれか | コンテナ内のクラスターファイルパス。両方ある場合はこちらを優先 |
| `PORT` | いいえ | コンテナ内の待ち受けポート（既定 `8080`） |
| `TREND_FILE` | いいえ | 24 時間の読み書きトレンドバッファのパス |
| `CLUSTERS_FILE` | いいえ | 複数クラスター一覧の保存パス |
| `AUTH` | いいえ | `on` / `off`（既定は off）。on のときはユーザーとパスワードが必要 |
| `AUTH_USER` | 認証時 | ログインユーザー名（最大 64） |
| `AUTH_PASSWORD` | 認証時 | ログインパスワード（最大 256） |

ログインを有効にする例:

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  -e AUTH=on \
  -e AUTH_USER=admin \
  -e AUTH_PASSWORD=secret \
  runu/foundationdb-dashboard:7.3.62
```

## ページ

### 概要

![概要](docs/imgs/overview.png)

クラスターの可用性、データ状態、リカバリ、冗長性、ストレージエンジン、コーディネーター、プロセスとロール、読み書き負荷、レイテンシ、故障耐性。データは `\xff\xff/status/json` から取得し、タブが表示されている間は約 3 秒ごとに更新します。グラフはバックグラウンドで 5 秒ごとにサンプリングし、約 24 時間保持します。

### データ閲覧

![データ閲覧](docs/imgs/data.png)

左にディレクトリツリー、中央にキー表（キー、値のプレビュー、種類、サイズ）、右に詳細。一覧を下にスクロールすると続きを読み込みます。

検索欄には hex だけでなく tuple テキストを書けます:

```text
"M"
("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")
```

文字列、整数、小数、`true` / `false` / `null`、`0x` バイト、`uuid(...)`、`float32(...)`、`Versionstamp(...)`、入れ子の tuple に対応します。プレフィックスを書くとその下のキー、空ならユーザーキースペースを列挙します。

値は JSON、読めるテキスト、tuple、バイナリの順で判定します。4 バイトと 8 バイトのリトルエンディアン整数には短い注記が付きます。全文と折りたたみ可能な hex は詳細パネルにあります。

書き込み:

- 正確なキーの新規作成・編集・名前変更
- 1 件または複数選択した正確なキーの削除（確認必須。サーバーは `confirm: true` を要求）
- インポート / エクスポート、バックアップ / 復元
- キーを監視し、値が変わったら更新

範囲削除はありません。

### クラスター構成

![クラスター構成](docs/imgs/configuration.png)

保存した接続の追加・切り替え・削除。環境変数由来の組み込み接続は削除できません。切り替え時にトレンドバッファはクリアされます。

**接続中**のクラスターについて、冗長性、ストレージエンジン、および 4 種類のロール数（commit proxies、GRV proxies、logs、resolvers）を変更できます。コーディネーター、リージョン、暗号化、テナントは引き続き `fdbcli` を使います。

## ビルドとテスト

```bash
make test
```

`libfdb_c` を必要としないパッケージを実行します。Docker イメージのビルドでは `go test ./...` が走り、`internal/fdbstore`（linux + cgo）も含みます。

```bash
make build
make up
```

アプリのバージョンはイメージビルド時に埋め込まれます（`APP_VERSION`、既定 `1.0.0`）。

## ライセンス

本プロジェクトは [MIT License](LICENSE) です。

本プロジェクトは非公式の FoundationDB クライアント UI です。FoundationDB 本体は Apple のライセンスに従ってください。
