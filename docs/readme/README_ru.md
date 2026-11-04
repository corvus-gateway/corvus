<div align="center">

[English](../../README.md) · [한국어](README_ko.md) · [简体中文](README_zh.md) · [日本語](README_ja.md) · **Русский** · [Français](README_fr.md) · [Deutsch](README_de.md) · [Español](README_es.md)

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

Corvus — это самостоятельно размещаемый шлюз, который устанавливается перед вашими веб-сервисами. Он так же прост в запуске, как Nginx Proxy Manager: домены, проброс трафика и бесплатный SSL настраиваются в несколько кликов. Кроме того, он **наблюдает, защищает и контролирует** проходящий трафик: мониторинг в реальном времени, блокировка атак, ограничение частоты запросов и очередь доступа на случай всплесков нагрузки. Всё работает на вашем собственном сервере; трафик не отправляется в сторонние облака.

- [Быстрая установка](#быстрая-установка)
- [Полная установка](#полная-установка)
- [Скриншоты](#скриншоты)

## Цель проекта

У небольших команд обычно получается так: обратный прокси для TLS, отдельный WAF, который никто никогда не настраивает, и полное непонимание происходящего в момент запуска продукта или атаки. Corvus объединяет всё это в одном решении с безопасными настройками по умолчанию: базовое обнаружение атак работает сразу после добавления хоста, без единого написанного правила, а все расширенные возможности остаются опциональными. Цель — шлюз, который можно установить за пять минут и которому по-прежнему можно доверять в тот день, когда трафик вырастет в десять раз.

Corvus — не форк Nginx Proxy Manager. Это новая реализация (плоскость данных на OpenResty, плоскость управления на Go, консоль на React), которая умеет **импортировать существующую конфигурацию NPM**.

## Возможности

**Прокси и TLS**
- Прокси-хосты с поддержкой wildcard, WebSocket, HTTP/2, принудительного HTTPS и собственных фрагментов Nginx
- Бесплатные сертификаты Let's Encrypt (HTTP-01 или DNS-01 через Cloudflare) с автоматическим продлением, либо загрузка собственных
- Реальный IP клиента за Cloudflare или доверенными прокси; опциональная защита паролем для каждого хоста
- Каждое изменение проверяется командой `openresty -t`, применяется атомарно и может быть откачено

**Corvus Sentinel — модуль безопасности**
- Правила безопасности: сопоставление по IP, списку IP, стране, пути, строке запроса, методу или заголовку → блокировать, разрешить или записать в журнал, глобально или для отдельного хоста
- Проверка тела запроса для JSON, форм и multipart (журналирование или блокировка)
- **Виртуальные патчи** для свежих CVE в один клик (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Управляемые обновления сигнатур, принимаемые только если они подписаны и новее текущих (без подмены и без отката версии)
- Автоматический бан IP-адресов повторных нарушителей с эскалацией и исключениями
- Ограничение частоты запросов с режимом пробного прогона (dry-run); блокировка путей эксплойтов из коробки

**Очередь доступа**
- Пропускает посетителей в пределах заданной ёмкости, а остальных справедливо ставит в очередь во время всплесков
- Скорость допуска, политика fail-open / fail-closed, актуальное число активных и ожидающих посетителей, экстренное снятие ограничений
- Страница ожидания локализована на 8 языков

**Мониторинг и оповещения**
- Запросы в секунду, коды статусов, задержки p50/p95/p99, разбивка по хостам и путям — в реальном времени
- Поиск по журналу запросов и экспорт в CSV/JSON, события безопасности, постоянная история со сроком хранения и ротацией журналов
- Оповещения в Slack, Mattermost, Discord, Teams, Telegram или через подписанные webhook
- Prometheus `/metrics`

**Администрирование**
- Роли, MFA (TOTP + коды восстановления), единый вход OIDC, API-токены
- Белый список IP для консоли администратора, журнал аудита, история развёртываний
- По умолчанию SQLite или PostgreSQL; консоль администратора на 8 языках со светлой и тёмной темами

**Усиленная защита по умолчанию**
- Контейнеры без root-прав, с корневой файловой системой только для чтения и без Linux capabilities
- Redis защищён сгенерированным паролем и минимальным ACL
- OpenResty собирается из исходников с проверкой подписи

## Скриншоты

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="Панель мониторинга" width="900">
</p>

| Мониторинг трафика | Правила безопасности |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="Мониторинг трафика"> | <img src="../../assets/screenshots/security-rules.png" alt="Правила безопасности"> |
| **Прокси-хост** | **Очереди доступа** |
| <img src="../../assets/screenshots/proxy-host.png" alt="Прокси-хост"> | <img src="../../assets/screenshots/access-queues.png" alt="Очереди доступа"> |

## Быстрая установка

1. [Установите Docker](https://docs.docker.com/engine/install/) вместе с плагином Compose.
2. Создайте файл `docker-compose.yml` следующего вида:

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

Это вся установка: шлюз, консоль администратора и хранилище очереди доступа работают в одном
этом контейнере. Параметры описаны в разделе [Полная установка](#полная-установка).

3. Запустите стек командой

```bash
docker compose up -d
```

4. Прочитайте одноразовый токен настройки

```bash
docker compose logs corvus | grep setup_token
```

5. Войдите в консоль администратора по адресу `https://<your-server>:9443`, введите токен и создайте
   первого администратора. Пока вы не настроите собственный сертификат, консоль использует
   самоподписанный.

> [!TIP]
> Направьте DNS вашего домена на сервер, добавьте прокси-хост и запросите сертификат Let's Encrypt
> в той же форме.

### Обновление

```bash
docker compose pull
docker compose up -d
```

Настройки, сертификаты и история в `./data` сохраняются; изменения базы данных применяются
автоматически при запуске.

## Полная установка

### Порты

| Порт | Назначение |
|---|---|
| 80 / 443 | Шлюз — проксируемый трафик |
| 9443 | Консоль администратора и API — разрешайте доступ только из доверенных сетей (межсетевой экран или *Сеть → Сети с доступом к консоли администратора*) |

### Данные и резервные копии

Всё состояние хранится в `./data`: база данных настроек, история сгенерированной конфигурации,
сертификаты, история трафика и `secrets/`. **Делайте резервную копию `secrets/`** — там лежат ключи,
которыми шифруются секреты MFA и токены DNS в базе данных. Контейнер работает от непривилегированного
пользователя `10001` и при первом запуске становится владельцем `./data`.

### Усиленная установка

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml) запускает тот же образ с корневой
файловой системой только для чтения, без привилегий Linux (capabilities) и без повышения привилегий.
Сначала создайте каталог данных для пользователя `10001`:

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### Конфигурация

Задавайте эти переменные в разделе `environment:` файла `docker-compose.yml`.

| Переменная | Описание |
|---|---|
| `CORVUS_ADMIN_HOSTS` | Дополнительные имена/IP для сертификата консоли администратора, например `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | Использовать PostgreSQL вместо SQLite, например `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | Использовать внешний Redis/Valkey для очереди доступа (несколько шлюзов с общей очередью); вместе с `CORVUS_REDIS_PASSWORD`. По умолчанию: встроенный |
| `CORVUS_IMPORT_DIR` | Каталог с томами NPM, смонтированными только для чтения, для функции *Импорт из NPM* |
| `CORVUS_RULES_URL` | Необязательный URL подписанного пакета управляемых правил |
| `CORVUS_ADMIN_ALLOW_ANY=1` | Аварийный переключатель, если вы заблокировали себе доступ списком разрешённых адресов администратора |

### Миграция с Nginx Proxy Manager

Смонтируйте каталоги NPM `data` и `letsencrypt` только для чтения и укажите их Corvus:

```yml
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

Затем откройте в консоли *Импорт из NPM*. Хосты, сертификаты и списки доступа будут перенесены. После
этого остановите NPM, чтобы два сервиса не конкурировали за порты 80/443 и продление сертификатов.

## Разработка

```bash
npm --prefix web ci && npm --prefix web run build      # интерфейс администратора → встраивается в бинарный файл Go
go test ./control/...                                   # модульные тесты
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # только консоль, без OpenResty
```

| Путь | Содержимое |
|---|---|
| `gateway/` | Плоскость данных: OpenResty + Lua (правила, проверка тела запроса, очередь доступа) |
| `control/` | Плоскость управления (Go): база настроек, генерация конфигурации, развёртывание/откат, API |
| `web/` | Консоль администратора (React + Vite + TypeScript) |
| `deploy/` | Dockerfile, проверенная сборка OpenResty, файлы Compose, интеграционные тесты |
| `tools/` | Инструменты нагрузочного тестирования и подписи пакетов правил |

## Безопасность

О найденных уязвимостях сообщайте в частном порядке — см. [SECURITY.md](../../SECURITY.md).

## Лицензия

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon.
Данные соответствия IP-адресов странам предоставлены [DB-IP](https://db-ip.com) (CC BY 4.0). См. [NOTICE](../../NOTICE).
