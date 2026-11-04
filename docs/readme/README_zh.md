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
- 免费的 Let's Encrypt 证书（HTTP-01，或通过 Cloudflare 的 DNS-01），自动续期，也可上传自有证书
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

### 从 Nginx Proxy Manager 迁移

将 NPM 的 `data` 和 `letsencrypt` 目录以只读方式挂载，并让 Corvus 指向它们：

```yml
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

然后在控制台中打开 *从 NPM 导入*。主机、证书和访问列表都会被迁移过来。之后请停止 NPM，以免两者争用
80/443 端口和证书续期。

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
