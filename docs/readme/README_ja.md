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
- 自動更新される無料の Let's Encrypt 証明書(HTTP-01、または Cloudflare 経由の DNS-01)、または独自証明書のアップロード
- Cloudflare や信頼できるプロキシの背後でも実際のクライアント IP を取得、ホストごとのパスワード保護(任意)
- すべての変更は `openresty -t` で検証され、アトミックに適用され、ロールバックも可能

**Corvus Sentinel — セキュリティエンジン**
- セキュリティルール: IP、IP リスト、国、パス、クエリ、メソッド、ヘッダーで照合し、ブロック・許可・ログ記録。グローバルまたはホスト単位で適用
- JSON、フォーム、multipart のリクエストボディ検査(ログ記録またはブロック)
- 最新の CVE(React2Shell、wp2shell、PHP-CGI、Vite、n8n …)に対応するワンクリックの**仮想パッチ**
- 署名済みで、かつより新しい場合のみ受け入れるマネージド署名アップデート(改ざん・ダウングレードを防止)
- 繰り返し違反する IP の自動 BAN(段階的な厳格化と除外設定に対応)
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

### Nginx Proxy Manager からの移行

NPM の `data` と `letsencrypt` ディレクトリを読み取り専用でマウントし、Corvus から参照させます。

```yml
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

その後、コンソールで *NPM からインポート* を開きます。ホスト、証明書、アクセスリストが引き継がれます。
移行後は NPM を停止して、80/443 番ポートと証明書の更新を両者が奪い合わないようにしてください。

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
