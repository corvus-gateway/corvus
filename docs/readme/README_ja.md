<div align="center">

[English](../../README.md) · [한국어](README_ko.md) · [简体中文](README_zh.md) · **日本語** · [Русский](README_ru.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Español](README_es.md)

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../../assets/brand/corvus-logo-dark.svg">
  <img src="../../assets/brand/corvus-logo.svg" alt="Corvus" width="420">
</picture>

### Active Web Protection Gateway

<a href="https://github.com/corvus-gateway/corvus/releases/latest"><img src="https://img.shields.io/github/v/release/corvus-gateway/corvus?label=version&color=2ea44f&style=for-the-badge"></a>
<a href="../../LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg?style=for-the-badge"></a>
<a href="https://github.com/corvus-gateway/corvus/pkgs/container/gateway"><img src="https://img.shields.io/badge/ghcr.io-corvus--gateway%2Fgateway-24292f.svg?style=for-the-badge&logo=docker&logoColor=white"></a>
<a href="https://github.com/corvus-gateway/corvus/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/corvus-gateway/corvus/ci.yml?branch=main&label=CI&style=for-the-badge"></a>

<img src="https://img.shields.io/badge/OpenResty-1.31-00b140?style=flat-square">
<img src="https://img.shields.io/badge/Go-1.26-00add8?style=flat-square&logo=go&logoColor=white">
<img src="https://img.shields.io/badge/React-18-61dafb?style=flat-square&logo=react&logoColor=black">
<img src="https://img.shields.io/badge/i18n-8_languages-8250df?style=flat-square">
<img src="https://img.shields.io/badge/platform-amd64_|_arm64-555?style=flat-square&logo=docker&logoColor=white">
<a href="https://scorecard.dev/viewer/?uri=github.com/corvus-gateway/corvus"><img src="https://api.scorecard.dev/projects/github.com/corvus-gateway/corvus/badge?style=flat-square"></a>
<a href="https://www.bestpractices.dev/projects/16315"><img src="https://www.bestpractices.dev/projects/16315/badge"></a>
<a href="https://www.bestpractices.dev/projects/16315/baseline-1"><img src="https://www.bestpractices.dev/projects/16315/baseline"></a>

</div>

Corvus は、Web サービスの前段に置くセルフホスト型ゲートウェイです。Nginx Proxy Manager と同じくらい
手軽に運用でき、ドメイン、フォワーディング、無料 SSL を数クリックで設定できます。さらに、通過する
トラフィックを**監視し、保護し、制御**します。リアルタイム監視、攻撃のブロック、レート制限、
アクセス集中に備えたアクセスキューを備えています。すべてが自分のサーバー上で動作し、トラフィックが
サードパーティのクラウドに送信されることはありません。

- [クイックセットアップ](#クイックセットアップ)
- [詳細セットアップ](#詳細セットアップ)
- [スクリーンショット](#スクリーンショット)

## プロジェクトの目標

小規模なチームでは、TLS 用のリバースプロキシと、チューニングされないままの WAF を別々に運用し、
ローンチや攻撃の際に何が起きているのかも把握できない、という状況によく陥ります。Corvus はこれらを
安全なデフォルト設定とともに 1 つにまとめました。ホストを追加した時点で、ルールを 1 行も書かなくても
基本的な攻撃検知が動作し、高度なオプションはすべて任意です。5 分でインストールでき、トラフィックが
10 倍に膨らんだ日にも信頼できるゲートウェイを目指しています。

Corvus は Nginx Proxy Manager のフォークではありません。既存の NPM 設定を**インポートできる**、
新規実装(OpenResty のデータプレーン、Go のコントロールプレーン、React のコンソール)です。

## 主な機能

**プロキシと TLS**
- ワイルドカード、WebSocket、HTTP/2、HTTPS 強制、カスタム Nginx スニペットに対応したプロキシホスト
- リダイレクトホスト: ドメインを別のドメインへ転送(301/302/303/307/308、パスの維持も可能)。TLS と保護機能も同じように適用
- 自動更新される無料の Let's Encrypt 証明書(HTTP-01、または DNS-01: Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap)、または独自証明書のアップロード(更新時はそのまま置き換え可能)
- ストリーム: TCP/UDP ポート(データベース、SSH、MQTT、ゲームサーバー)を別のホストへ転送。TCP では TLS 終端も可能
- Cloudflare や信頼できるプロキシの背後でも実際のクライアント IP を取得、ホストごとのパスワード保護(任意)
- ホストごとのセキュリティヘッダーのプリセット(基本 / 厳格: nosniff、フレーム・リファラーポリシー、レポート専用 CSP、HttpOnly/SameSite/Secure Cookie フラグ)。アプリがすでに送っているヘッダーは上書きしません
- テキスト応答の Gzip 圧縮(既定で有効)、ホストごとのアップロードサイズ上限とアップストリームのタイムアウト
- すべての変更は `openresty -t` で検証され、アトミックに適用され、ロールバックも可能

**Corvus Sentinel — セキュリティエンジン**
- セキュリティルール: IP、IP リスト、国、パス、クエリ、メソッド、ヘッダー、User-Agent リストで照合し、ブロック・許可・ログ記録・チャレンジ。グローバルまたはホスト単位で適用
- URL から最新の状態に保たれる IP リストと User-Agent リスト(例: Tor の出口ノード、悪質なボットのリスト。プリセット付き)。バックグラウンドで更新され、内容が変わったときだけ再デプロイ
- 外部サービスを使わないブラウザチャレンジ: JavaScript のプルーフ・オブ・ワーク(難易度を調整可能)または軽量な Cookie チェック。署名付きで IP に紐づいた通過 Cookie を発行
- ホストごとの「攻撃対応モード」: すべての訪問者にチャレンジを課します。手動でオンにするか、1 秒あたりのリクエスト数またはブロックされたリクエストの割合がしきい値を超えている間は自動でオン(オン・オフ時に通知)
- JSON、フォーム、multipart のリクエストボディ検査(ログ記録またはブロック)
- 最新の CVE(React2Shell、wp2shell、PHP-CGI、Vite、n8n …)に対応するワンクリックの**仮想パッチ**
- 署名済みで、かつより新しい場合のみ受け入れるマネージド署名アップデート(改ざん・ダウングレードを防止)
- 繰り返し違反する IP の自動 BAN(段階的な厳格化と除外設定に対応)。不正な振る舞いに対する即時 BAN も可能(IP とサイトごとの 4xx 応答が多すぎる場合)
- ドライランモード付きのレート制限、標準で有効なエクスプロイトパスのブロック

**アクセスキュー(Access Queue)**
- 収容上限まで訪問者を入場させ、アクセスが集中した際は残りを公平に待機させます
- 入場レート、fail-open / fail-closed ポリシー、リアルタイムの入場中/待機中の人数、緊急解放
- 8 言語にローカライズされた待機ページ

**監視とアラート**
- リアルタイムのリクエスト数/秒、ステータスコード、p50/p95/p99 レイテンシ、ホスト別・パス別の内訳
- リクエストログの検索と CSV/JSON エクスポート、セキュリティイベント、保持期間とログローテーションを備えた永続的な履歴
- Slack、Mattermost、Discord、Teams、Telegram、または署名付き Webhook へのアラート通知
- Prometheus `/metrics`

**管理**
- ロール、MFA(TOTP + リカバリーコード)、OIDC シングルサインオン、API トークン
- 管理コンソールの IP 許可リスト、監査ログ、デプロイ履歴
- 標準は SQLite、PostgreSQL にも対応。ライト/ダークテーマ対応の 8 言語管理コンソール

**デフォルトから堅牢**
- 読み取り専用のルートファイルシステムで Linux ケーパビリティなし、非 root で動作するコンテナ
- 自動生成されたパスワードと最小限の ACL で保護された Redis
- 署名検証済みのソースからビルドした OpenResty

## スクリーンショット

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="Dashboard" width="900">
</p>

| トラフィック監視 | セキュリティルール |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="Traffic monitor"> | <img src="../../assets/screenshots/security-rules.png" alt="Security rules"> |
| **プロキシホスト** | **アクセスキュー** |
| <img src="../../assets/screenshots/proxy-host.png" alt="Proxy host"> | <img src="../../assets/screenshots/access-queues.png" alt="Access queues"> |

## クイックセットアップ

1. Compose プラグイン付きで [Docker をインストール](https://docs.docker.com/engine/install/)します。
2. 次のような `docker-compose.yml` ファイルを作成します。

```yml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
    volumes:
      - ./data:/data
```

これでインストールは完了です。ゲートウェイ、管理コンソール、アクセス待機列ストアは、この 1 つの
コンテナ内で動作します。オプションについては[詳細セットアップ](#詳細セットアップ)を参照してください。

3. 次のコマンドでスタックを起動します。

```bash
docker compose up -d
```

4. 一度だけ使用するセットアップトークンを確認します。

```bash
docker compose logs corvus | grep setup_token
```

5. `https://<your-server>:9443` で管理コンソールにログインし、トークンを入力して最初の管理者を作成します。
   独自の証明書を設定するまで、コンソールは自己署名証明書を使用します。

> [!TIP]
> ドメインの DNS をサーバーに向け、プロキシホストを追加して、同じフォームで Let's Encrypt 証明書を
> 取得してください。

### アップグレード

```bash
docker compose pull
docker compose up -d
```

`./data` 内の設定、証明書、履歴は保持されます。データベースの変更は起動時に自動的に適用されます。

## 詳細セットアップ

### ポート

| ポート | 用途 |
|---|---|
| 80 / 443 | ゲートウェイ — プロキシ対象のトラフィック |
| 9443 | 管理コンソールと API — 信頼できるネットワークのみに許可してください(ファイアウォール、または *ネットワーク → 管理コンソールへのアクセスを許可するネットワーク*) |

### データとバックアップ

すべての状態は `./data` に保存されます。設定データベース、生成された設定の履歴、証明書、トラフィック履歴、
そして `secrets/` です。**`secrets/` は必ずバックアップしてください。** データベース内の MFA シークレットと
DNS トークンを暗号化する鍵が含まれています。コンテナは権限のないユーザー `10001` で実行され、初回起動時に
`./data` の所有権を取得します。

**コンソールからのバックアップ:** *データ保持 → バックアップと復元 → バックアップをダウンロード*(管理者のみ、
ログイン中のセッションに限る — API トークンではバックアップをダウンロードできません)で、次の内容を含む
`corvus-backup-<version>-<timestamp>.tar.gz` が作成されます。

- `corvus.db` — SQLite 設定データベースの一貫したスナップショット(Corvus の稼働中に `VACUUM INTO` で
  取得)
- `secrets/`、`certs/`、`rules/`(インストール済みのマネージドルールバンドル)
- `manifest.json` — フォーマットバージョン、Corvus のバージョン、作成日時、データベースエンジン、
  すべてのファイルの SHA-256

生成された設定バージョン(`nginx/`)、トラフィック履歴とリクエストログ(`history/`)、国データベース
(`geoip/`)、`run/` は含まれません。これらは再生成されるか、再ダウンロードされます。PostgreSQL
(`CORVUS_DB_URL`)の場合、アーカイブにはデータベースは含まれず `README.txt` だけが入ります。データベースは
`pg_dump` でバックアップし、ダンプをアーカイブと一緒に保管してください(暗号化されたフィールドには
`secrets/data_key` が必要です)。**アーカイブには秘密鍵とデータキーが含まれるため、暗号化して保管して
ください。** ダウンロードはすべて監査ログに記録されます。

**コンソールからの復元:** *バックアップから復元* でアーカイブ(最大 200 MB)をアップロードします。
フォーマットバージョン、すべてのファイルの SHA-256、絶対パス・`..`・リンク・不明なエントリがないこと、
データベースエンジンが同じことが検査され、概要(バージョン、日付、ファイル数と証明書数)が表示されます。
*次回の再起動時に復元* を押すとアーカイブが `./data/restore-pending/` に準備されます。コンテナを再起動
するまでは何も変わりません。

```bash
docker compose restart
```

起動時、データベースを開く前に、Corvus は現在の `corvus.db`、`secrets/`、`certs/`、`rules/`、`nginx/` を
`./data/restore-backup-<timestamp>/` に移動し、その場所にバックアップを配置します(その後、復元された
データベースから設定が再生成されます)。元に戻すには、コンテナを停止してこれらのファイルを戻してください。
準備されたファイルが検査に通らなかった場合、復元は適用されずに現在の状態が保たれ、準備されたファイルは
`./data/restore-failed-<timestamp>/` に移動されます(`docker compose logs` を参照)。トラフィック履歴は
復元の影響を受けません。

**手動での復元:** アーカイブの構成はデータディレクトリの構成と同じです。初回起動の前に、空の `./data` に
展開してください(余分な `manifest.json` は問題ありません)。

```bash
mkdir data && tar -xzf corvus-backup-….tar.gz -C data
docker compose up -d
```

### ハードニング済みインストール

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml) は、同じイメージを読み取り専用のルート
ファイルシステム、Linux ケーパビリティなし、権限昇格なしで実行します。先にユーザー `10001` 用の
データディレクトリを作成してください。

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### 設定

これらは `docker-compose.yml` の `environment:` に設定します。

| 変数 | 説明 |
|---|---|
| `CORVUS_ADMIN_HOSTS` | 管理用証明書に追加する名前/IP。例: `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | SQLite の代わりに PostgreSQL を使用。例: `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | アクセス待機列に外部の Redis/Valkey を使用(複数のゲートウェイで 1 つの待機列を共有)。`CORVUS_REDIS_PASSWORD` と併用。デフォルト: 内蔵 |
| `CORVUS_IMPORT_DIR` | *NPM からインポート*で使用する、読み取り専用の NPM ボリュームを置いたディレクトリ |
| `CORVUS_RULES_URL` | 署名付きマネージドルールバンドルの URL(任意) |
| `CORVUS_ADMIN_ALLOW_ANY=1` | 管理者の許可リストが原因でアクセスできなくなった場合の復旧用スイッチ |
| `CORVUS_REDIS_PORT` | 127.0.0.1 で動作する内蔵アクセスキューストアのポート(デフォルト `6379`)。`network_mode: host` でホストがすでに 6379 を使っている場合に変更 |
| `CORVUS_RESOLVER` | 転送先ホストの名前解決に使う DNS サーバー。デフォルト: Docker の DNS、ホストネットワークでは `/etc/resolv.conf` の最初の `nameserver` |
| `CORVUS_LOOPBACK=container` | `127.0.0.1`/`localhost` の転送先ホストが Docker ホストではなくコンテナ自身を指すようにする |
| `CORVUS_IPV6` | `auto`(既定): システムに IPv6 があれば IPv6 でも待ち受け · `off`: IPv4 のみ · `on`: 常に IPv6 で待ち受け、転送先ホストも IPv6 アドレスで解決(IPv6 専用アップストリーム) |
| `PUID` / `PGID` | コンテナを実行し `./data` を所有するユーザーとグループ(デフォルト `10001`)。例: NAS のユーザー |
| `CORVUS_ACME_DIRECTORY` | Let's Encrypt の代わりに使う別の CA の ACME ディレクトリ URL。例: プライベート step-ca の `https://ca.internal/acme/acme/directory` |
| `LEGO_CA_CERTIFICATES` | ACME クライアントが信頼するプライベート CA の PEM ファイル(コンテナにマウント)。例: `/data/ca.pem` |

### 転送先ホスト

プロキシホストの転送先には次を指定できます。

- **別のコンテナ**を名前で指定。例: `xoshop-web` ポート `80`。Corvus がそのコンテナと Docker
  ネットワークを共有している必要があります(`corvus` サービスにそのネットワークを追加)。
- **プライベートまたはパブリック IP**。例: `10.3.2.39` ポート `8084`。
- **Docker ホスト自体のサービス**を、NPM と同様に `127.0.0.1` または `localhost` で指定。コンテナ内では
  これらは Docker ホストを意味するため、`host.docker.internal` は不要です。サービスはコンテナから到達できる
  アドレス(`127.0.0.1` だけでなく `0.0.0.0` または Docker ブリッジ)で待ち受ける必要があり、ホストの
  ファイアウォールで Docker ネットワークからの接続を許可する必要があります。

ホストの `127.0.0.1` でのみ待ち受けるサービスに接続する場合や、`network_mode: host` を使っていた NPM の
構成をそのまま使う場合は、Corvus をホストネットワークで実行します。`ports:` と `networks:` を
`network_mode: host` に置き換えてください(ホストで 6379 が使用中なら `CORVUS_REDIS_PORT` も設定)。

### ストリーム (TCP/UDP)

*ストリーム* は TCP / UDP ポートを転送先ホストへ転送します(ルールは上記と同じ)。TCP では登録済みの
証明書で TLS を終端することもできます。デフォルトのブリッジネットワークでは、各ストリームのポートを
`docker-compose.yml` にも公開し、`docker compose up -d` で反映してください。

```yaml
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
      - '5432:5432'       # ストリーム: PostgreSQL (TCP)
      - '1194:1194/udp'   # ストリーム: OpenVPN (UDP)
```

`network_mode: host` では追加の作業は不要です。Corvus 自身が使うポート(80、443、9443 と内部ポート)は
ストリームに使えません。

### Nginx Proxy Manager からの移行

コンソールの *NPM からインポート* は、既存の NPM 環境から **プロキシホスト、リダイレクトホスト、ストリーム、証明書**
（Cloudflare DNS トークンを含む Let's Encrypt 証明書とカスタム証明書）と **アクセスリスト**（ユーザーと
IP ルール）をコピーします。NPM のファイルとデータベースを読み取るだけで NPM 自体は変更しないため、いつでも元に戻せます。

インポートされないもの: 404 ホスト、カスタムロケーション、NPM 固有の変数を
使う高度な設定。スキップされる項目はすべてプレビューに一覧表示されます。

**NPM が使っているデータベースを確認してください。** NPM の `docker-compose.yml` を開きます。

- `DB_MYSQL_*` / `DB_POSTGRES_*` 変数がなく、`data/database.sqlite` がある → **SQLite**（NPM の既定）
- `DB_MYSQL_HOST`、`DB_MYSQL_USER`、… があり、通常は別の `db` サービスもある → **MySQL / MariaDB**
- `DB_POSTGRES_*` → PostgreSQL はインポートに対応していないため、コンソールでホストを作り直してください

どちらの場合も、NPM の `data` と `letsencrypt` ディレクトリを Corvus に読み取り専用でマウントする必要が
あります（証明書ファイルはここにあります）。

```yml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

NPM の `./data` と `./letsencrypt` ボリュームのホスト側パスを指定してください。NPM と Corvus はどちらも
80/443 番ポートを使うため、同時には実行できません。

#### SQLite を使う NPM（既定）

```bash
cd /path/to/npm && docker compose down        # NPM を停止（80/443 を解放）
cd /path/to/corvus && docker compose up -d     # 上記のマウントで Corvus を起動
```

コンソールで *NPM からインポート* を開き、**SQLite** のまま *プレビュー* をクリックし、インポートする
項目を選んで *選択項目をインポート* をクリックします。プレビューで `permission denied` と表示された場合は、
Corvus（uid 10001 で動作）がファイルを読めるようにしてください: `sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`。

#### MySQL / MariaDB を使う NPM

インポート中は Corvus が NPM のデータベースに接続する必要があるため、NPM のプロキシサービスだけを停止し、
データベースは動かしたままにします。

```bash
cd /path/to/npm && docker compose stop app     # 80/443 を使う NPM サービス（多くは "app"）
cd /path/to/corvus && docker compose up -d

# インポートのために Corvus を NPM の Docker ネットワークに接続
docker network ls | grep -i npm                # 例: npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

*NPM からインポート* で **MySQL / MariaDB** を選び、NPM のデータベース設定を入力します。ホスト =
データベースのサービス名（通常は `db`）、ポート `3306`、そして `DB_MYSQL_NAME`、`DB_MYSQL_USER`、
`DB_MYSQL_PASSWORD` の値です。接続は読み取り専用で、パスワードは保存されません。その後
*プレビュー* → *選択項目をインポート* と進みます。

インポートが終わったら:

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # NPM のデータベースも停止
```

#### インポート後

- インポートしたホストを確認したら、`CORVUS_IMPORT_DIR` と 2 つの `/import` マウントを削除し、
  もう一度 `docker compose up -d` を実行します。
- 証明書は Corvus で引き続き更新されます。未対応の DNS プロバイダーを使っていたホストは自動更新なしで
  インポートされ、プレビューで警告が表示されます。
- **元に戻す:** Corvus で `docker compose down` を実行し、次に NPM で `docker compose up -d` を実行します。

## パフォーマンス

GitHub ホストの `ubuntu-24.04` ランナー(4 vCPU AMD EPYC 9V45)で [ベンチマークワークフロー](../benchmark.md)
を使って測定しました。Corvus は **2 vCPU** に固定し、wrk とバックエンドは残りの 2 つで実行、keep-alive 接続
64 本で、3 バイトの応答を返す `GET /` を送っています。

| シナリオ | req/s | p50 | p99 | メモリ |
|---|---:|---:|---:|---:|
| nginx 1.28 リバースプロキシ(ベースライン) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus、デフォルトのホスト | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus、ルール 30 件 + 1,000 CIDR のリスト + エクスプロイトブロック + ヘッダー + レート制限 | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus、HTTPS(ECDSA P-256)、keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus、HTTPS、リクエストごとに新しい TLS ハンドシェイク | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus、アクセスキュー(入場済みの訪問者) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus、チャレンジルール(通過 Cookie を持つ訪問者) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

基本のリクエスト処理では、Corvus は素の nginx のおよそ 4 分の 3 のスループットを出します(アップストリーム
への keep-alive 接続プール、ワーカーごとにまとめて送るアクセスログ)。保護機能一式(1,000 CIDR のリストを
含むルール 30 件、エクスプロイトブロック、セキュリティヘッダー、レート制限)を有効にすると、そこから約 28 %
下がります。**注意:** 負荷生成ツールが同じマシン上で動く共有 CI ランナーで、応答もごく小さいため、数値は
控えめなものです。シナリオやバージョンの比較を目的としたもので、処理能力を保証するものではありません。
メモリにはコントロールプレーンと内蔵の Valkey が含まれます。

## 開発

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only, no OpenResty
```

| パス | 内容 |
|---|---|
| `gateway/` | データプレーン: OpenResty + Lua(ルール、ボディ検査、アクセスキュー) |
| `control/` | コントロールプレーン(Go): 設定 DB、設定生成、デプロイ/ロールバック、API |
| `web/` | 管理コンソール(React + Vite + TypeScript) |
| `deploy/` | Dockerfile、検証済み OpenResty ビルド、Compose ファイル、結合テスト |
| `tools/` | 負荷テストおよびルールバンドル署名ツール |

## セキュリティ

脆弱性は非公開でご報告ください。詳細は [SECURITY.md](../../SECURITY.md) をご覧ください。

## ライセンス

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon.
IP から国への対応データは [DB-IP](https://db-ip.com)(CC BY 4.0)によるものです。[NOTICE](../../NOTICE) を参照してください。
