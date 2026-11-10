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

<p align="center"><img src="../../assets/demo/corvus-demo.gif" alt="Corvus demo" width="900"></p>

- [快速安装](#快速安装)
- [文档](https://corvusgateway.org/)
- [截图](#截图)

## 项目目标

小团队往往会同时拥有：负责 TLS 的反向代理、从不调优的独立 WAF，以及在产品发布或遭受攻击时完全不知道发生了什么的窘境。Corvus 将这些功能集成在一个盒子里，并提供安全的默认设置：添加主机后，基础的攻击检测立即生效，无需编写任何规则，所有高级选项均为可选。我们的目标是打造一个五分钟即可安装完成、并且在流量暴增十倍的那一天依然值得信赖的网关。

Corvus 不是 Nginx Proxy Manager 的分支。它是全新的实现（OpenResty 数据平面、Go 控制平面、React 控制台），并且可以**导入现有的 NPM 配置**。

## 功能特性

**代理与 TLS**
- 代理主机支持通配符、WebSocket、HTTP/2、强制 HTTPS 以及自定义 Nginx 片段
- 为主机和流添加备注，记下每一项的用途（可搜索）
- 自定义位置：按路径（如 `/api`）转发到不同的上游，并保留主机的全部防护
- 重定向主机：将域名跳转到另一个域名（301/302/303/307/308，可选保留路径），同样享有 TLS 与防护
- 免费的 Let's Encrypt 证书（HTTP-01，或DNS-01：Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap，服务商凭据保存后可重复使用），自动续期，也可上传自有证书（续期后可原地替换）
- 流转发：将 TCP/UDP 端口（数据库、SSH、MQTT、游戏服务器）转发到其他主机，TCP 可选 TLS 终止
- 未知主机名可选择返回 404 页面、关闭连接或重定向
- 在 Cloudflare 或受信任代理之后获取真实客户端 IP；可按主机启用密码保护
- 按主机设置安全响应头预设（基础 / 严格：nosniff、frame 与 referrer 策略、仅报告的 CSP、HttpOnly/SameSite/Secure Cookie 标志），绝不覆盖应用自身已发送的响应头
- 文本响应的 Gzip 压缩（默认开启），以及按主机设置的上传大小限制和上游超时
- 每次变更都会通过 `openresty -t` 验证，原子化应用，并且可以回滚
- 有新的 Corvus 版本发布时，控制台会提示并链接到发布说明（可关闭）

**Corvus Sentinel — 安全引擎**
- 安全规则：匹配 IP、IP 列表、国家/地区、路径、查询参数、方法、请求头或 User-Agent 列表 → 拦截、放行、仅记录或质询，可全局生效或按主机生效
- 从 URL 保持更新的 IP 列表和 User-Agent 列表（例如 Tor 出口节点、恶意爬虫列表；内置预设），在后台刷新，仅在内容变化时重新部署
- 无需第三方服务的浏览器质询：JavaScript 工作量证明（难度可调）或轻量级 Cookie 检查，并签发经签名、与 IP 绑定的通行 Cookie
- 按主机的“遭受攻击模式”：对所有访客发起质询——可手动开启，也可在每秒请求数或被拦截请求的比例超过阈值期间自动开启（开启和关闭时发送告警）
- 对 JSON、表单和 multipart 请求体进行检测（记录或拦截）
- 针对近期 CVE（React2Shell、wp2shell、PHP-CGI、Vite、n8n 等）的一键**虚拟补丁**
- 托管特征库更新，仅接受已签名且版本更新的内容（防篡改、防降级）
- 对屡次攻击的 IP 自动封禁，支持逐级升级与豁免；可选对不良行为立即封禁（同一 IP 在同一站点产生过多 4xx 响应）
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
[文档](https://corvusgateway.org/)。

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

## 文档

配置、加固安装、目标主机、流、备份、从 Nginx Proxy Manager 迁移、安全规则、访问队列和故障排查，
请参见 **[文档](https://corvusgateway.org/)**(英文、韩文)。

韩文的分步教程书见 [WikiDocs](https://wikidocs.net/book/22096)。

## 性能

使用 [基准测试工作流](../benchmark.md) 在 GitHub 托管的 `ubuntu-24.04` 运行器（4 vCPU AMD EPYC 9V45）上测得：
Corvus 绑定到 **2 vCPU**，wrk 和后端运行在另外两个 vCPU 上，64 个 keep-alive 连接，请求 `GET /`，响应为 3 字节。

| 场景 | req/s | p50 | p99 | 内存 |
|---|---:|---:|---:|---:|
| nginx 1.28 反向代理（基线） | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus，默认主机 | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus，30 条规则 + 1,000 个 CIDR 的列表 + 漏洞利用拦截 + 响应头 + 速率限制 | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus，HTTPS（ECDSA P-256），keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus，HTTPS，每个请求都进行新的 TLS 握手 | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus，访问排队（已放行的访客） | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus，质询规则（持有通行 Cookie 的访客） | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

在基础请求路径上，Corvus 可达到原生 nginx 约四分之三的吞吐量（到上游的 keep-alive 连接池、按 worker
批量发送的访问日志）；启用完整防护（含 1,000 个 CIDR 列表的 30 条规则、漏洞利用拦截、安全响应头和速率
限制）后，在此基础上再降低约 28 %。**注意：** 测试使用的是共享 CI 运行器，负载生成器
与被测服务在同一台机器上，且响应极小——这些数字偏保守，用于比较不同场景和版本，而非容量保证。内存包括
控制平面和内置的 Valkey。

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
