<div align="center">

[English](../../README.md) · [한국어](README_ko.md) · **简体中文** · [日本語](README_ja.md) · [Русский](README_ru.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Español](README_es.md)

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

Corvus 是部署在您的 Web 服务前端的自托管网关。它与 Nginx Proxy Manager 一样易于运行——只需点几下即可配置域名、转发和免费 SSL——同时还能对流经的流量进行**监视、防护和管控**：实时监控、攻击拦截、速率限制，以及应对流量高峰的访问排队。一切都运行在您自己的服务器上，不会有任何流量发送到第三方云。

- [快速安装](#快速安装)
- [完整安装](#完整安装)
- [截图](#截图)

## 项目目标

小团队往往会同时拥有：负责 TLS 的反向代理、从不调优的独立 WAF，以及在产品发布或遭受攻击时完全不知道发生了什么的窘境。Corvus 将这些功能集成在一个盒子里，并提供安全的默认设置：添加主机后，基础的攻击检测立即生效，无需编写任何规则，所有高级选项均为可选。我们的目标是打造一个五分钟即可安装完成、并且在流量暴增十倍的那一天依然值得信赖的网关。

Corvus 不是 Nginx Proxy Manager 的分支。它是全新的实现（OpenResty 数据平面、Go 控制平面、React 控制台），并且可以**导入现有的 NPM 配置**。

## 功能特性

**代理与 TLS**
- 代理主机支持通配符、WebSocket、HTTP/2、强制 HTTPS 以及自定义 Nginx 片段
- 免费的 Let's Encrypt 证书（HTTP-01，或DNS-01：Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap），自动续期，也可上传自有证书
- 在 Cloudflare 或受信任代理之后获取真实客户端 IP；可按主机启用密码保护
- 每次变更都会通过 `openresty -t` 验证，原子化应用，并且可以回滚

**Corvus Sentinel — 安全引擎**
- 安全规则：匹配 IP、IP 列表、国家/地区、路径、查询参数、方法或请求头 → 拦截、放行或仅记录，可全局生效或按主机生效
- 对 JSON、表单和 multipart 请求体进行检测（记录或拦截）
- 针对近期 CVE（React2Shell、wp2shell、PHP-CGI、Vite、n8n 等）的一键**虚拟补丁**
- 托管特征库更新，仅接受已签名且版本更新的内容（防篡改、防降级）
- 对屡次攻击的 IP 自动封禁，支持逐级升级与豁免
- 带演练（dry-run）模式的速率限制；开箱即用的漏洞利用路径拦截

**访问排队**
- 在容量范围内放行访客，并在流量高峰期间公平地让其余访客排队
- 放行速率、故障放行 / 故障拒绝（fail-open / fail-closed）策略、实时活跃/等待人数、紧急放行
- 等待页面支持 8 种语言

**监控与告警**
- 实时请求数/秒、状态码、p50/p95/p99 延迟，以及按主机和路径的细分
- 请求日志搜索与 CSV/JSON 导出、安全事件、带保留期限的持久化历史记录和日志轮转
- 告警可发送至 Slack、Mattermost、Discord、Teams、Telegram 或带签名的 webhook
- Prometheus `/metrics`

**管理**
- 角色、MFA（TOTP + 恢复码）、OIDC 单点登录、API 令牌
- 管理控制台 IP 白名单、审计日志、部署历史
- 默认使用 SQLite，也可使用 PostgreSQL；管理控制台支持 8 种语言及浅色/深色主题

**默认加固**
- 非 root 容器，只读根文件系统，且不授予任何 Linux capabilities
- Redis 使用自动生成的密码和最小化 ACL 进行保护
- OpenResty 从经过签名验证的源码构建

## 截图

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="仪表盘" width="900">
</p>

| 流量监控 | 安全规则 |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="流量监控"> | <img src="../../assets/screenshots/security-rules.png" alt="安全规则"> |
| **代理主机** | **访问排队** |
| <img src="../../assets/screenshots/proxy-host.png" alt="代理主机"> | <img src="../../assets/screenshots/access-queues.png" alt="访问排队"> |

## 快速安装

1. 安装带有 Compose 插件的 [Docker](https://docs.docker.com/engine/install/)。
2. 创建如下所示的 `docker-compose.yml` 文件：

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

这就是全部安装内容：网关、管理控制台和访问队列存储都运行在这一个容器中。更多选项请参见
[完整安装](#完整安装)。

3. 运行以下命令启动服务：

```bash
docker compose up -d
```

4. 读取一次性设置令牌：

```bash
docker compose logs corvus | grep setup_token
```

5. 在 `https://<your-server>:9443` 登录管理控制台，输入令牌并创建第一个管理员。在配置自己的证书之前，
   控制台使用自签名证书。

> [!TIP]
> 将域名的 DNS 指向该服务器，添加代理主机，并在同一表单中申请 Let's Encrypt 证书。

### 升级

```bash
docker compose pull
docker compose up -d
```

`./data` 中的设置、证书和历史记录都会保留；数据库变更会在启动时自动应用。

## 完整安装

### 端口

| 端口 | 用途 |
|---|---|
| 80 / 443 | 网关——代理流量 |
| 9443 | 管理控制台与 API——仅允许受信任的网络访问（防火墙，或 *网络 → 允许访问管理控制台的网络*） |

### 数据与备份

所有状态都保存在 `./data` 中：设置数据库、生成的配置历史、证书、流量历史和 `secrets/`。
**请备份 `secrets/`**——其中保存着用于加密数据库中 MFA 密钥和 DNS 令牌的密钥。容器以非特权用户
`10001` 运行，并在首次启动时接管 `./data` 的所有权。

### 加固安装

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml) 使用相同的镜像运行，但根文件系统为只读，
不授予任何 Linux capability，也禁止提权。请先为用户 `10001` 创建数据目录：

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### 配置

在 `docker-compose.yml` 的 `environment:` 下设置这些变量。

| 变量 | 说明 |
|---|---|
| `CORVUS_ADMIN_HOSTS` | 管理控制台证书的额外名称/IP，例如 `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | 使用 PostgreSQL 代替 SQLite，例如 `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | 为访问队列使用外部 Redis/Valkey（多个网关共享同一个队列）；需配合 `CORVUS_REDIS_PASSWORD`。默认：内置 |
| `CORVUS_IMPORT_DIR` | 包含只读 NPM 卷的目录，用于 *从 NPM 导入* |
| `CORVUS_RULES_URL` | 已签名托管规则包的 URL（可选） |
| `CORVUS_ADMIN_ALLOW_ANY=1` | 因管理员允许列表而无法访问时使用的恢复开关 |
| `CORVUS_REDIS_PORT` | 内置访问队列存储在 127.0.0.1 上的端口（默认 `6379`）；使用 `network_mode: host` 且主机已占用 6379 时请修改 |
| `CORVUS_RESOLVER` | 用于解析转发目标主机的 DNS 服务器。默认：Docker 的 DNS；使用主机网络时为 `/etc/resolv.conf` 中的第一个 `nameserver` |
| `CORVUS_LOOPBACK=container` | 让 `127.0.0.1`/`localhost` 转发目标指向容器自身，而不是 Docker 主机 |

### 转发目标主机

代理主机可以转发到：

- **另一个容器**，按名称指定，例如 `xoshop-web` 端口 `80`——Corvus 必须与其共享一个 Docker 网络
  （将该网络添加到 `corvus` 服务）；
- **私有或公网 IP**，例如 `10.3.2.39` 端口 `8084`；
- **Docker 主机本身上的服务**，与 NPM 一样使用 `127.0.0.1` 或 `localhost`。在容器内，这些地址指的是
  Docker 主机，因此无需 `host.docker.internal`。该服务必须监听容器可访问的地址（`0.0.0.0` 或 Docker
  网桥，而不仅仅是 `127.0.0.1`），并且主机防火墙必须允许来自 Docker 网络的连接。

如需访问只监听主机 `127.0.0.1` 的服务，或希望原样保留使用 `network_mode: host` 的 NPM 配置，请以主机网络
运行 Corvus：将 `ports:` 和 `networks:` 替换为 `network_mode: host`（如果主机上 6379 已被占用，还需设置
`CORVUS_REDIS_PORT`）。

### 从 Nginx Proxy Manager 迁移

控制台中的 *从 NPM 导入* 会从现有的 NPM 安装中复制 **代理主机、证书**（Let's Encrypt 证书和自定义证书，
包括 Cloudflare DNS 令牌）以及 **访问列表**（用户和 IP 规则）。它只读取 NPM 的文件和数据库，不会修改 NPM
本身，因此随时可以切换回去。

不会导入的内容：流（TCP/UDP）、重定向主机、404 主机、自定义位置，以及使用 NPM 专有变量的高级配置。
所有被跳过的项目都会在预览中列出。

**先确认你的 NPM 使用哪种数据库。** 打开 NPM 的 `docker-compose.yml`：

- 没有 `DB_MYSQL_*` / `DB_POSTGRES_*` 变量，且存在 `data/database.sqlite` → **SQLite**（NPM 默认）
- 有 `DB_MYSQL_HOST`、`DB_MYSQL_USER`、…，通常还有一个单独的 `db` 服务 → **MySQL / MariaDB**
- `DB_POSTGRES_*` → 导入不支持 PostgreSQL，请在控制台中重新创建主机

两种情况都需要将 NPM 的 `data` 和 `letsencrypt` 目录以只读方式挂载到 Corvus 中（证书文件位于其中）：

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

请使用 NPM 的 `./data` 和 `./letsencrypt` 卷在宿主机上的路径。NPM 和 Corvus 都需要 80/443 端口，
因此不能同时运行。

#### 使用 SQLite 的 NPM（默认）

```bash
cd /path/to/npm && docker compose down        # 停止 NPM（释放 80/443）
cd /path/to/corvus && docker compose up -d     # 使用上述挂载启动 Corvus
```

在控制台中打开 *从 NPM 导入*，保持 **SQLite**，点击 *预览*，选择要导入的内容后点击 *导入所选项*。
如果预览报告 `permission denied`，请让 Corvus（以 uid 10001 运行）能够读取这些文件：`sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`。

#### 使用 MySQL / MariaDB 的 NPM

导入期间 Corvus 需要连接 NPM 的数据库，因此只停止 NPM 的代理服务，保持其数据库继续运行：

```bash
cd /path/to/npm && docker compose stop app     # 占用 80/443 的 NPM 服务（通常为 "app"）
cd /path/to/corvus && docker compose up -d

# 为导入将 Corvus 连接到 NPM 的 Docker 网络
docker network ls | grep -i npm                # 例如 npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

在 *从 NPM 导入* 中选择 **MySQL / MariaDB**，并填写 NPM 的数据库设置：主机 = 数据库服务名（通常为 `db`），
端口 `3306`，以及 `DB_MYSQL_NAME`、`DB_MYSQL_USER` 和 `DB_MYSQL_PASSWORD` 的值。连接为只读，密码不会被
保存。然后依次点击 *预览* → *导入所选项*。

导入完成后：

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # 同时停止 NPM 的数据库
```

#### 导入之后

- 检查导入的主机，然后删除 `CORVUS_IMPORT_DIR` 和两个 `/import` 挂载，再次运行
  `docker compose up -d`。
- 证书会继续在 Corvus 中续期。使用了不受支持的 DNS 提供商的主机会在没有自动续期的情况下导入，预览中
  会对此发出警告。
- **切换回去：** 在 Corvus 中执行 `docker compose down`，然后在 NPM 中执行 `docker compose up -d`。

## 开发

```bash
npm --prefix web ci && npm --prefix web run build      # 管理界面 → 嵌入到 Go 二进制文件中
go test ./control/...                                   # 单元测试
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # 仅控制台，不含 OpenResty
```

| 路径 | 内容 |
|---|---|
| `gateway/` | 数据平面：OpenResty + Lua（规则、请求体检测、访问排队） |
| `control/` | 控制平面（Go）：设置数据库、配置生成、部署/回滚、API |
| `web/` | 管理控制台（React + Vite + TypeScript） |
| `deploy/` | Dockerfile、经验证的 OpenResty 构建、Compose 文件、集成测试 |
| `tools/` | 压力测试与规则包签名工具 |

## 安全

请通过私密渠道报告漏洞——参见 [SECURITY.md](../../SECURITY.md)。

## 许可证

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon。
IP 到国家/地区数据由 [DB-IP](https://db-ip.com) 提供（CC BY 4.0）。参见 [NOTICE](../../NOTICE)。
