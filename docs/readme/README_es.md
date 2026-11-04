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
<a href="https://www.bestpractices.dev/projects/16315/baseline-1"><img src="https://www.bestpractices.dev/projects/16315/baseline"></a>

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
- Hosts de redirección: envíe dominios a otro dominio (301/302/303/307/308, conservando opcionalmente la ruta), con el mismo TLS y la misma protección
- Certificados gratuitos de Let's Encrypt (HTTP-01 o DNS-01 mediante Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap) con renovación automática, o suba los suyos
- IP real del cliente detrás de Cloudflare o de proxies de confianza; protección opcional con contraseña por host
- Ajustes predefinidos de cabeceras de seguridad por host (Básico / Estricto: nosniff, políticas de frame y de referrer, CSP en modo solo informe, atributos de cookie HttpOnly/SameSite/Secure), sin sobrescribir nunca lo que la aplicación ya envía
- Compresión gzip de las respuestas de texto (activada por defecto), y límite de tamaño de subida y tiempo de espera del upstream por host
- Cada cambio se valida con `openresty -t`, se aplica de forma atómica y se puede revertir

**Corvus Sentinel — motor de seguridad**
- Reglas de seguridad: coincidencia por IP, lista de IP, país, ruta, query, método, cabecera o lista de User-Agent → bloquear, permitir, registrar o desafiar, de forma global o por host
- Listas de IP y de User-Agent mantenidas al día desde una URL (p. ej. nodos de salida de Tor, listas de bots maliciosos; ajustes predefinidos incluidos), actualizadas en segundo plano y redesplegadas solo cuando cambian
- Desafío del navegador sin servicios de terceros: una prueba de trabajo en JavaScript (dificultad ajustable) o una comprobación ligera por cookie, con una cookie de paso firmada y vinculada a la IP
- Modo «bajo ataque» por host: desafía a todos los visitantes; se activa a mano o automáticamente mientras las solicitudes por segundo o la proporción de solicitudes bloqueadas superan un umbral (con alertas al activarse y desactivarse)
- Inspección del cuerpo de las solicitudes JSON, de formulario y multipart (registrar o bloquear)
- **Parches virtuales** con un clic para CVE recientes (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Actualizaciones de firmas gestionadas, aceptadas solo si están firmadas y son más recientes (sin manipulación ni degradación)
- Bloqueo automático de IP reincidentes, con escalado y exenciones; bloqueo inmediato opcional por mal comportamiento (demasiadas respuestas 4xx por IP y sitio)
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

**Copia de seguridad desde la consola:** *Retención de datos → Copia de seguridad y restauración → Descargar
copia* (administradores, solo con sesión iniciada; los tokens de API no pueden descargar copias) crea
`corvus-backup-<version>-<timestamp>.tar.gz` con:

- `corvus.db`: una instantánea coherente de la base de datos de ajustes SQLite (tomada con `VACUUM INTO`
  mientras Corvus sigue en marcha),
- `secrets/`, `certs/` y `rules/` (paquete de reglas gestionadas instalado),
- `manifest.json`: versión del formato, versión de Corvus, fecha de creación, motor de base de datos y el
  SHA-256 de cada archivo.

Quedan fuera las versiones de configuración generadas (`nginx/`), el historial de tráfico y los registros de
solicitudes (`history/`), la base de datos de países (`geoip/`) y `run/`: se regeneran o se vuelven a
descargar. Con PostgreSQL (`CORVUS_DB_URL`) el archivo no contiene ninguna base de datos, solo un `README.txt`;
haga la copia de la base de datos con `pg_dump` y guarde el volcado junto con el archivo (sus campos cifrados
necesitan `secrets/data_key`). **El archivo contiene las claves privadas y la clave de datos: guárdelo
cifrado.** Cada descarga queda registrada en el registro de auditoría.

**Restauración desde la consola:** *Restaurar desde una copia* sube un archivo (200 MB como máximo). Se
comprueba (versión del formato, SHA-256 de cada archivo, sin rutas absolutas, `..`, enlaces ni entradas
desconocidas, mismo motor de base de datos) y se muestra su resumen (versión, fecha, número de archivos y de
certificados). *Restaurar en el próximo reinicio* lo prepara en `./data/restore-pending/`; nada cambia hasta
que reinicie el contenedor:

```bash
docker compose restart
```

Al iniciarse, antes de abrir la base de datos, Corvus mueve los `corvus.db`, `secrets/`, `certs/`, `rules/` y
`nginx/` actuales a `./data/restore-backup-<timestamp>/` y coloca la copia en su lugar (después la configuración
se vuelve a generar a partir de la base de datos restaurada). Para revertir, detenga el contenedor y devuelva
esos archivos a su sitio. Si los archivos preparados no superan las comprobaciones, la restauración no se aplica,
se conserva el estado actual y los archivos preparados se mueven a `./data/restore-failed-<timestamp>/`
(consulte `docker compose logs`). Una restauración no toca el historial de tráfico.

**Restauración manual:** la estructura del archivo es la del directorio de datos. Antes del primer inicio,
extráigalo en un `./data` vacío (el `manifest.json` adicional no causa problemas):

```bash
mkdir data && tar -xzf corvus-backup-….tar.gz -C data
docker compose up -d
```

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
| `CORVUS_REDIS_PORT` | Puerto del almacén integrado de la cola de acceso en 127.0.0.1 (por defecto `6379`); cámbielo con `network_mode: host` si el host ya usa el 6379 |
| `CORVUS_RESOLVER` | Servidor DNS para los hosts de destino. Por defecto: el DNS de Docker, o el primer `nameserver` de `/etc/resolv.conf` con red de host |
| `CORVUS_LOOPBACK=container` | Hacer que los hosts de destino `127.0.0.1`/`localhost` apunten al propio contenedor en lugar de al host de Docker |
| `CORVUS_IPV6` | `auto` (predeterminado): escuchar también en IPv6 si el sistema lo admite · `off`: solo IPv4 · `on`: escuchar siempre en IPv6 y resolver también los hosts de destino a direcciones IPv6 (upstreams solo IPv6) |
| `PUID` / `PGID` | Usuario y grupo con los que se ejecuta el contenedor y que son propietarios de `./data` (por defecto `10001`), p. ej. su usuario del NAS |
| `CORVUS_ACME_DIRECTORY` | URL del directorio ACME de otra CA en lugar de Let's Encrypt, p. ej. un step-ca privado `https://ca.internal/acme/acme/directory` |
| `LEGO_CA_CERTIFICATES` | Archivo(s) PEM de una CA privada en la que debe confiar el cliente ACME (móntelo en el contenedor), p. ej. `/data/ca.pem` |

### Hosts de destino

Un host proxy puede reenviar a:

- **otro contenedor** por su nombre, p. ej. `xoshop-web` puerto `80`; Corvus debe compartir una red Docker
  con él (añada esa red al servicio `corvus`);
- **una IP privada o pública**, p. ej. `10.3.2.39` puerto `8084`;
- **un servicio en el propio host de Docker** como `127.0.0.1` o `localhost`, igual que en NPM. Dentro de un
  contenedor estas direcciones se refieren al host de Docker, por lo que no hace falta `host.docker.internal`.
  El servicio debe escuchar en una dirección accesible desde el contenedor (`0.0.0.0` o el puente de Docker, no
  solo `127.0.0.1`), y el cortafuegos del host debe permitir las conexiones desde las redes Docker.

Para llegar a servicios que solo escuchan en `127.0.0.1` del host, o para mantener sin cambios una instalación
de NPM que usaba `network_mode: host`, ejecute Corvus con red de host: sustituya `ports:` y `networks:` por
`network_mode: host` (y defina `CORVUS_REDIS_PORT` si el 6379 ya está ocupado en el host).

### Migración desde Nginx Proxy Manager

*Importar desde NPM* en la consola copia los **hosts proxy, hosts de redirección, certificados**
(Let's Encrypt y personalizados, incluidos los tokens DNS de Cloudflare) y **listas de acceso** (usuarios y reglas IP) de
una instalación existente de NPM. Solo lee los archivos y la base de datos de NPM; NPM no se modifica, así
que puede volver a él en cualquier momento.

No se importan: streams (TCP/UDP), hosts 404, ubicaciones personalizadas ni la configuración avanzada
que usa variables propias de NPM. La vista previa enumera todo lo que se omite.

**Compruebe qué base de datos usa su NPM.** Abra el `docker-compose.yml` de NPM:

- sin variables `DB_MYSQL_*` / `DB_POSTGRES_*` y existe `data/database.sqlite` → **SQLite** (predeterminada en NPM)
- `DB_MYSQL_HOST`, `DB_MYSQL_USER`, … y normalmente un servicio `db` aparte → **MySQL / MariaDB**
- `DB_POSTGRES_*` → la importación no admite PostgreSQL; vuelva a crear los hosts en la consola

En ambos casos hay que montar los directorios `data` y `letsencrypt` de NPM como solo lectura en Corvus
(ahí están los archivos de certificados):

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

Use las rutas del host de los volúmenes `./data` y `./letsencrypt` de NPM. NPM y Corvus necesitan los
puertos 80/443, por lo que no pueden ejecutarse a la vez.

#### NPM con SQLite (predeterminada)

```bash
cd /path/to/npm && docker compose down        # detener NPM (libera 80/443)
cd /path/to/corvus && docker compose up -d     # iniciar Corvus con los montajes anteriores
```

En la consola, abra *Importar desde NPM*, deje **SQLite**, haga clic en *Vista previa*, elija qué importar y
haga clic en *Importar selección*. Si la vista previa indica `permission denied`, haga que Corvus pueda leer
los archivos (se ejecuta como uid 10001): `sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`.

#### NPM con MySQL / MariaDB

Corvus tiene que acceder a la base de datos de NPM durante la importación, así que detenga solo el servicio
proxy de NPM y mantenga su base de datos en ejecución:

```bash
cd /path/to/npm && docker compose stop app     # el servicio de NPM que ocupa 80/443 (a menudo "app")
cd /path/to/corvus && docker compose up -d

# conectar Corvus a la red Docker de NPM para la importación
docker network ls | grep -i npm                # p. ej. npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

En *Importar desde NPM*, elija **MySQL / MariaDB** e introduzca la configuración de la base de datos de NPM:
host = nombre del servicio de base de datos (normalmente `db`), puerto `3306` y los valores de
`DB_MYSQL_NAME`, `DB_MYSQL_USER` y `DB_MYSQL_PASSWORD`. La conexión es de solo lectura y la contraseña no se
guarda. Después, *Vista previa* → *Importar selección*.

Cuando termine la importación:

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # detener también la base de datos de NPM
```

#### Después de la importación

- Revise los hosts importados; después elimine `CORVUS_IMPORT_DIR` y los dos montajes `/import` y vuelva a
  ejecutar `docker compose up -d`.
- Los certificados se siguen renovando en Corvus. Los hosts que usaban un proveedor DNS no compatible se
  importan sin renovación automática; la vista previa avisa de ellos.
- **Volver atrás:** `docker compose down` en Corvus y después `docker compose up -d` en NPM.

## Rendimiento

Medido con el [flujo de trabajo de benchmark](../benchmark.md) en un runner `ubuntu-24.04` alojado por GitHub
(4 vCPU AMD EPYC 9V45): Corvus fijado a **2 vCPU**, wrk y el backend en las otras dos, 64 conexiones
keep-alive, `GET /` con una respuesta de 3 bytes.

| Escenario | req/s | p50 | p99 | Memoria |
|---|---:|---:|---:|---:|
| nginx 1.28 como proxy inverso (referencia) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus, host predeterminado | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus, 30 reglas + lista de 1,000 CIDR + bloqueo de exploits + cabeceras + límite de tasa | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus, HTTPS (ECDSA P-256), keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus, HTTPS, nuevo handshake TLS en cada solicitud | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus, cola de acceso (visitantes admitidos) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus, regla de desafío (visitantes con pase) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

En la ruta básica de la solicitud, Corvus alcanza alrededor de tres cuartas partes del rendimiento de nginx
sin más (pools de conexiones keep-alive al upstream, registro de acceso agrupado por worker); el conjunto
completo de protección — 30 reglas con una lista de 1,000 CIDR, bloqueo de exploits, cabeceras de seguridad y
límite de tasa — cuesta alrededor de un 28 % adicional. **Advertencia:** se trata de un
runner de CI compartido, con el generador de carga en la misma máquina y respuestas diminutas; las cifras son
conservadoras y sirven para comparar escenarios y versiones, no como garantía de capacidad. La memoria incluye
el plano de control y el Valkey integrado.

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
