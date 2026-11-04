<div align="center">

[English](../../README.md) · [한국어](README_ko.md) · [简体中文](README_zh.md) · [日本語](README_ja.md) · [Русский](README_ru.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · **Español**

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

Corvus es un gateway autoalojado que se sitúa delante de sus servicios web. Es tan fácil de usar
como Nginx Proxy Manager — dominios, reenvío y SSL gratuito en unos pocos clics — y además
**vigila, protege y controla** el tráfico que lo atraviesa: monitorización en vivo, bloqueo de
ataques, limitación de tasa y una cola de acceso para los picos de tráfico. Todo se ejecuta en su
propio servidor; ningún tráfico se envía a una nube de terceros.

- [Instalación rápida](#instalación-rápida)
- [Instalación completa](#instalación-completa)
- [Capturas de pantalla](#capturas-de-pantalla)

## Objetivo del proyecto

Los equipos pequeños suelen acabar con un proxy inverso para TLS, un WAF aparte que nunca ajustan y
ninguna idea de lo que ocurre cuando llega un lanzamiento o un ataque. Corvus lo reúne todo en una
sola pieza con valores predeterminados seguros: la detección básica de ataques funciona en cuanto
se añade un host, sin escribir una sola regla, y cada opción avanzada sigue siendo opcional. El
objetivo es un gateway que se instala en cinco minutos y en el que aún se puede confiar el día en
que el tráfico se multiplique por diez.

Corvus no es un fork de Nginx Proxy Manager. Es una implementación nueva (plano de datos con
OpenResty, plano de control en Go, consola en React) que puede **importar una configuración de NPM existente**.

## Características

**Proxy y TLS**
- Hosts proxy con comodines, WebSocket, HTTP/2, HTTPS forzado y fragmentos de Nginx personalizados
- Certificados gratuitos de Let's Encrypt (HTTP-01 o DNS-01 mediante Cloudflare) con renovación automática, o suba los suyos
- IP real del cliente detrás de Cloudflare o de proxies de confianza; protección opcional con contraseña por host
- Cada cambio se valida con `openresty -t`, se aplica de forma atómica y se puede revertir

**Corvus Sentinel — motor de seguridad**
- Reglas de seguridad: coincidencia por IP, lista de IP, país, ruta, query, método o cabecera → bloquear, permitir o registrar, de forma global o por host
- Inspección del cuerpo de las solicitudes JSON, de formulario y multipart (registrar o bloquear)
- **Parches virtuales** con un clic para CVE recientes (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Actualizaciones de firmas gestionadas, aceptadas solo si están firmadas y son más recientes (sin manipulación ni degradación)
- Bloqueo automático de IP reincidentes, con escalado y exenciones
- Límites de tasa con modo de simulación (dry-run); bloqueo de rutas de exploits listo para usar

**Cola de acceso**
- Admite visitantes hasta una capacidad determinada y pone en cola al resto de forma equitativa durante los picos
- Tasa de admisión, política fail-open / fail-closed, contadores en vivo de activos/en espera, liberación de emergencia
- Página de espera localizada en 8 idiomas

**Monitorización y alertas**
- Solicitudes/s en vivo, códigos de estado, latencia p50/p95/p99, desglose por host y por ruta
- Búsqueda en el registro de solicitudes y exportación a CSV/JSON, eventos de seguridad, historial persistente con retención y rotación de registros
- Alertas a Slack, Mattermost, Discord, Teams, Telegram o webhooks firmados
- `/metrics` de Prometheus

**Administración**
- Roles, MFA (TOTP + códigos de recuperación), inicio de sesión único con OIDC, tokens de API
- Lista de IP permitidas para la consola de administración, registro de auditoría, historial de despliegues
- SQLite por defecto o PostgreSQL; consola de administración en 8 idiomas con temas claro y oscuro

**Reforzado por defecto**
- Contenedores sin root, con sistema de archivos raíz de solo lectura y sin capacidades de Linux
- Redis protegido con una contraseña generada y una ACL mínima
- OpenResty compilado a partir de fuentes con firma verificada

## Capturas de pantalla

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="Panel" width="900">
</p>

| Monitor de tráfico | Reglas de seguridad |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="Monitor de tráfico"> | <img src="../../assets/screenshots/security-rules.png" alt="Reglas de seguridad"> |
| **Host proxy** | **Colas de acceso** |
| <img src="../../assets/screenshots/proxy-host.png" alt="Host proxy"> | <img src="../../assets/screenshots/access-queues.png" alt="Colas de acceso"> |

## Instalación rápida

1. [Instale Docker](https://docs.docker.com/engine/install/) con el plugin Compose.
2. Cree un archivo `docker-compose.yml` como este:

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

Esta es toda la instalación: la pasarela, la consola de administración y el almacén de la cola de acceso se
ejecutan en este único contenedor. Consulte [Instalación completa](#instalación-completa) para ver las opciones.

3. Levante su stack ejecutando

```bash
docker compose up -d
```

4. Lea el token de configuración de un solo uso

```bash
docker compose logs corvus | grep setup_token
```

5. Inicie sesión en la consola de administración en `https://<your-server>:9443`, introduzca el token y cree el
   primer administrador. La consola usa un certificado autofirmado hasta que configure el suyo.

> [!TIP]
> Apunte el DNS de su dominio al servidor, añada un host proxy y solicite un certificado de Let's Encrypt en
> el mismo formulario.

### Actualización

```bash
docker compose pull
docker compose up -d
```

Los ajustes, certificados e historial de `./data` se conservan; los cambios en la base de datos se aplican
automáticamente al iniciar.

## Instalación completa

### Puertos

| Puerto | Finalidad |
|---|---|
| 80 / 443 | Pasarela — tráfico proxificado |
| 9443 | Consola de administración y API — permita solo redes de confianza (firewall, o *Red → Redes permitidas para la consola de administración*) |

### Datos y copias de seguridad

Todo el estado se guarda en `./data`: la base de datos de ajustes, el historial de configuración generada,
los certificados, el historial de tráfico y `secrets/`. **Haga copia de seguridad de `secrets/`**: contiene las
claves que cifran los secretos MFA y los tokens DNS de la base de datos. El contenedor se ejecuta como el
usuario sin privilegios `10001` y toma posesión de `./data` en el primer inicio.

### Instalación reforzada

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml) ejecuta la misma imagen con un sistema
de archivos raíz de solo lectura, sin capabilities de Linux y sin escalada de privilegios. Cree primero el
directorio de datos para el usuario `10001`:

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### Configuración

Defina estas variables en `environment:` dentro de `docker-compose.yml`.

| Variable | Descripción |
|---|---|
| `CORVUS_ADMIN_HOSTS` | Nombres/IP adicionales para el certificado de administración, p. ej. `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | Usar PostgreSQL en lugar de SQLite, p. ej. `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | Usar un Redis/Valkey externo para la cola de acceso (varias pasarelas compartiendo una cola); junto con `CORVUS_REDIS_PASSWORD`. Por defecto: integrado |
| `CORVUS_IMPORT_DIR` | Directorio con los volúmenes de NPM de solo lectura para *Importar desde NPM* |
| `CORVUS_RULES_URL` | URL opcional de un paquete de reglas gestionadas firmado |
| `CORVUS_ADMIN_ALLOW_ANY=1` | Interruptor de recuperación si se ha bloqueado con la lista de permitidos de administración |

### Migración desde Nginx Proxy Manager

Monte los directorios `data` y `letsencrypt` de NPM como solo lectura y haga que Corvus los use:

```yml
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

Después abra *Importar desde NPM* en la consola. Se trasladan los hosts, certificados y listas de acceso.
Detenga NPM a continuación para que ambos no compitan por los puertos 80/443 ni por las renovaciones de
certificados.

## Desarrollo

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only, no OpenResty
```

| Ruta | Contenido |
|---|---|
| `gateway/` | Plano de datos: OpenResty + Lua (reglas, inspección del cuerpo, cola de acceso) |
| `control/` | Plano de control (Go): base de datos de ajustes, generación de configuración, despliegue/rollback, API |
| `web/` | Consola de administración (React + Vite + TypeScript) |
| `deploy/` | Dockerfile, compilación verificada de OpenResty, archivos Compose, pruebas de integración |
| `tools/` | Herramientas de pruebas de carga y de firma de paquetes de reglas |

## Seguridad

Informe de las vulnerabilidades de forma privada — consulte [SECURITY.md](../../SECURITY.md).

## Licencia

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon.
Datos de IP a país de [DB-IP](https://db-ip.com) (CC BY 4.0). Consulte [NOTICE](../../NOTICE).
