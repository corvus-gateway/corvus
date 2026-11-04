<div align="center">

[English](../../README.md) · [한국어](README_ko.md) · [简体中文](README_zh.md) · [日本語](README_ja.md) · [Русский](README_ru.md) · [Français](README_fr.md) · **Deutsch** · [Español](README_es.md)

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

Corvus ist ein selbst gehostetes Gateway, das vor Ihren Webdiensten sitzt. Es ist so einfach zu
betreiben wie Nginx Proxy Manager — Domains, Weiterleitung und kostenloses SSL mit wenigen Klicks —
und **überwacht, schützt und steuert** zusätzlich den durchlaufenden Datenverkehr: Live-Monitoring,
Angriffsabwehr, Ratenbegrenzung und eine Zugriffswarteschlange für Lastspitzen. Alles läuft auf
Ihrem eigenen Server; es wird kein Traffic an eine Drittanbieter-Cloud gesendet.

- [Schnellstart](#schnellstart)
- [Vollständige Einrichtung](#vollständige-einrichtung)
- [Screenshots](#screenshots)

## Projektziel

Kleine Teams landen meist bei einem Reverse-Proxy für TLS, einer separaten WAF, die nie
abgestimmt wird, und keiner Ahnung, was passiert, wenn ein Launch oder ein Angriff eintrifft.
Corvus vereint das in einer Lösung mit sicheren Standardeinstellungen: Die grundlegende
Angriffserkennung funktioniert, sobald ein Host hinzugefügt wird – ohne eine einzige Regel zu
schreiben –, und jede erweiterte Option bleibt optional. Das Ziel ist ein Gateway, das sich in fünf
Minuten installieren lässt und dem man auch dann noch vertraut, wenn der Traffic auf das Zehnfache steigt.

Corvus ist kein Fork von Nginx Proxy Manager. Es ist eine Neuentwicklung (OpenResty-Datenebene,
Go-Steuerungsebene, React-Konsole), die ein **bestehendes NPM-Setup importieren** kann.

## Funktionen

**Proxy & TLS**
- Proxy-Hosts mit Wildcards, WebSocket, HTTP/2, erzwungenem HTTPS und benutzerdefinierten Nginx-Snippets
- Kostenlose Let's-Encrypt-Zertifikate (HTTP-01 oder DNS-01 über Cloudflare) mit automatischer Erneuerung oder eigene Zertifikate per Upload
- Echte Client-IP hinter Cloudflare oder vertrauenswürdigen Proxys; optionaler Passwortschutz pro Host
- Jede Änderung wird mit `openresty -t` geprüft, atomar angewendet und kann zurückgerollt werden

**Corvus Sentinel — Sicherheits-Engine**
- Sicherheitsregeln: Abgleich nach IP, IP-Liste, Land, Pfad, Query, Methode oder Header → blockieren, erlauben oder protokollieren, global oder pro Host
- Prüfung des Request-Bodys für JSON-, Formular- und Multipart-Inhalte (protokollieren oder blockieren)
- **Virtuelle Patches** per Klick für aktuelle CVEs (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Verwaltete Signatur-Updates, die nur akzeptiert werden, wenn sie signiert und neuer sind (keine Manipulation, kein Downgrade)
- Automatische IP-Sperren für Wiederholungstäter, mit Eskalation und Ausnahmen
- Ratenbegrenzungen mit Dry-Run-Modus; Blockierung von Exploit-Pfaden ab Werk

**Zugriffswarteschlange**
- Besucher bis zu einer festgelegten Kapazität zulassen und den Rest bei Lastspitzen fair in die Warteschlange stellen
- Zulassungsrate, Fail-open-/Fail-closed-Richtlinie, Live-Zähler für aktive/wartende Besucher, Notfall-Freigabe
- Warteseite in 8 Sprachen lokalisiert

**Monitoring & Alarme**
- Live-Anfragen/s, Statuscodes, p50/p95/p99-Latenz, Aufschlüsselung nach Host und Pfad
- Suche im Request-Log und CSV/JSON-Export, Sicherheitsereignisse, persistente Historie mit Aufbewahrung und Log-Rotation
- Alarme an Slack, Mattermost, Discord, Teams, Telegram oder signierte Webhooks
- Prometheus-`/metrics`

**Administration**
- Rollen, MFA (TOTP + Wiederherstellungscodes), OIDC-Single-Sign-On, API-Tokens
- IP-Allowlist für die Admin-Konsole, Audit-Log, Deployment-Historie
- Standardmäßig SQLite oder PostgreSQL; Admin-Konsole in 8 Sprachen mit hellem und dunklem Design

**Standardmäßig gehärtet**
- Container ohne Root-Rechte, mit schreibgeschütztem Root-Dateisystem und ohne Linux-Capabilities
- Redis ist durch ein generiertes Passwort und eine minimale ACL geschützt
- OpenResty wird aus signaturgeprüften Quellen gebaut

## Screenshots

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="Dashboard" width="900">
</p>

| Traffic-Monitor | Sicherheitsregeln |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="Traffic-Monitor"> | <img src="../../assets/screenshots/security-rules.png" alt="Sicherheitsregeln"> |
| **Proxy-Host** | **Zugriffswarteschlangen** |
| <img src="../../assets/screenshots/proxy-host.png" alt="Proxy-Host"> | <img src="../../assets/screenshots/access-queues.png" alt="Zugriffswarteschlangen"> |

## Schnellstart

1. [Installieren Sie Docker](https://docs.docker.com/engine/install/) mit dem Compose-Plugin.
2. Erstellen Sie eine Datei `docker-compose.yml` wie diese:

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

Das ist die gesamte Installation: Gateway, Admin-Konsole und Speicher für die Zugriffswarteschlange laufen in
diesem einen Container. Optionen finden Sie unter [Vollständige Einrichtung](#vollständige-einrichtung).

3. Starten Sie Ihren Stack mit

```bash
docker compose up -d
```

4. Lesen Sie das einmalige Einrichtungs-Token aus

```bash
docker compose logs corvus | grep setup_token
```

5. Melden Sie sich unter `https://<your-server>:9443` an der Admin-Konsole an, geben Sie das Token ein und legen
   Sie den ersten Administrator an. Die Konsole verwendet ein selbstsigniertes Zertifikat, bis Sie ein eigenes
   einrichten.

> [!TIP]
> Richten Sie den DNS-Eintrag Ihrer Domain auf den Server, fügen Sie einen Proxy-Host hinzu und fordern Sie im
> selben Formular ein Let's-Encrypt-Zertifikat an.

### Aktualisieren

```bash
docker compose pull
docker compose up -d
```

Einstellungen, Zertifikate und der Verlauf in `./data` bleiben erhalten; Datenbankänderungen werden beim
Start automatisch angewendet.

## Vollständige Einrichtung

### Ports

| Port | Zweck |
|---|---|
| 80 / 443 | Gateway — Proxy-Datenverkehr |
| 9443 | Admin-Konsole und API — nur für vertrauenswürdige Netzwerke freigeben (Firewall oder *Netzwerk → Zugelassene Netze für die Admin-Konsole*) |

### Daten und Backups

Der gesamte Zustand liegt in `./data`: die Einstellungsdatenbank, der Verlauf der generierten Konfiguration,
Zertifikate, der Traffic-Verlauf und `secrets/`. **Sichern Sie `secrets/`** — dort liegen die Schlüssel, mit denen
MFA-Secrets und DNS-Tokens in der Datenbank verschlüsselt werden. Der Container läuft als unprivilegierter
Benutzer `10001` und übernimmt beim ersten Start den Besitz von `./data`.

### Gehärtete Installation

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml) startet dasselbe Image mit einem
schreibgeschützten Root-Dateisystem, ohne Linux-Capabilities und ohne Rechteausweitung. Legen Sie zuerst das
Datenverzeichnis für den Benutzer `10001` an:

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### Konfiguration

Setzen Sie diese Variablen unter `environment:` in `docker-compose.yml`.

| Variable | Beschreibung |
|---|---|
| `CORVUS_ADMIN_HOSTS` | Zusätzliche Namen/IPs für das Admin-Zertifikat, z. B. `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | PostgreSQL statt SQLite verwenden, z. B. `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | Externes Redis/Valkey für die Zugriffswarteschlange verwenden (mehrere Gateways teilen sich eine Warteschlange); zusammen mit `CORVUS_REDIS_PASSWORD`. Standard: integriert |
| `CORVUS_IMPORT_DIR` | Verzeichnis mit schreibgeschützten NPM-Volumes für *Import aus NPM* |
| `CORVUS_RULES_URL` | Optionale URL eines signierten Bundles mit verwalteten Regeln |
| `CORVUS_ADMIN_ALLOW_ANY=1` | Notfallschalter, falls Sie sich mit der Admin-Zulassungsliste ausgesperrt haben |
| `CORVUS_REDIS_PORT` | Port des integrierten Speichers der Zugriffswarteschlange auf 127.0.0.1 (Standard `6379`); bei `network_mode: host` ändern, wenn der Host 6379 bereits belegt |
| `CORVUS_RESOLVER` | DNS-Server für Ziel-Hosts. Standard: der DNS von Docker bzw. bei Host-Networking der erste `nameserver` aus `/etc/resolv.conf` |
| `CORVUS_LOOPBACK=container` | Ziel-Hosts `127.0.0.1`/`localhost` zeigen auf den Container selbst statt auf den Docker-Host |

### Ziel-Hosts

Ein Proxy-Host kann weiterleiten an:

- **einen anderen Container** über seinen Namen, z. B. `xoshop-web` Port `80` — Corvus muss sich mit ihm
  ein Docker-Netzwerk teilen (fügen Sie dieses Netzwerk dem Dienst `corvus` hinzu);
- **eine private oder öffentliche IP**, z. B. `10.3.2.39` Port `8084`;
- **einen Dienst auf dem Docker-Host selbst** als `127.0.0.1` oder `localhost`, wie in NPM. Innerhalb eines
  Containers bezeichnen diese Adressen den Docker-Host, `host.docker.internal` ist also nicht nötig. Der Dienst
  muss auf einer für den Container erreichbaren Adresse lauschen (`0.0.0.0` oder die Docker-Bridge, nicht nur
  `127.0.0.1`), und eine Host-Firewall muss Verbindungen aus Docker-Netzwerken zulassen.

Um Dienste zu erreichen, die nur auf `127.0.0.1` des Hosts lauschen, oder um ein NPM-Setup mit
`network_mode: host` unverändert zu übernehmen, betreiben Sie Corvus mit Host-Networking: Ersetzen Sie
`ports:` und `networks:` durch `network_mode: host` (und setzen Sie `CORVUS_REDIS_PORT`, falls 6379 auf dem
Host belegt ist).

### Migration von Nginx Proxy Manager

*Import aus NPM* in der Konsole übernimmt **Proxy-Hosts, Zertifikate** (Let's Encrypt und eigene,
einschließlich Cloudflare-DNS-Tokens) und **Zugriffslisten** (Benutzer und IP-Regeln) aus einer
bestehenden NPM-Installation. Dabei werden nur die Dateien und die Datenbank von NPM gelesen – NPM selbst
bleibt unverändert, Sie können also jederzeit zurückwechseln.

Nicht importiert werden: Streams (TCP/UDP), Weiterleitungs-Hosts, 404-Hosts, benutzerdefinierte Locations
und erweiterte Konfiguration mit NPM-spezifischen Variablen. Die Vorschau listet alles auf, was übersprungen
wird.

**Prüfen Sie, welche Datenbank Ihr NPM verwendet.** Öffnen Sie die `docker-compose.yml` von NPM:

- keine `DB_MYSQL_*`- / `DB_POSTGRES_*`-Variablen und `data/database.sqlite` vorhanden → **SQLite** (NPM-Standard)
- `DB_MYSQL_HOST`, `DB_MYSQL_USER`, … und meist ein separater `db`-Dienst → **MySQL / MariaDB**
- `DB_POSTGRES_*` → PostgreSQL wird vom Import nicht unterstützt; legen Sie die Hosts in der Konsole neu an

In beiden Fällen müssen die Verzeichnisse `data` und `letsencrypt` von NPM schreibgeschützt in Corvus
eingebunden werden (dort liegen die Zertifikatsdateien):

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

Verwenden Sie die Host-Pfade der NPM-Volumes `./data` und `./letsencrypt`. NPM und Corvus benötigen beide
die Ports 80/443 und können daher nicht gleichzeitig laufen.

#### NPM mit SQLite (Standard)

```bash
cd /path/to/npm && docker compose down        # NPM stoppen (gibt 80/443 frei)
cd /path/to/corvus && docker compose up -d     # Corvus mit den obigen Mounts starten
```

Öffnen Sie in der Konsole *Import aus NPM*, belassen Sie **SQLite**, klicken Sie auf *Vorschau*, wählen Sie
aus, was importiert werden soll, und klicken Sie auf *Auswahl importieren*. Meldet die Vorschau
`permission denied`, machen Sie die Dateien für Corvus lesbar (es läuft als uid 10001): `sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`.

#### NPM mit MySQL / MariaDB

Corvus muss die NPM-Datenbank während des Imports erreichen. Stoppen Sie daher nur den Proxy-Dienst von NPM
und lassen Sie die Datenbank weiterlaufen:

```bash
cd /path/to/npm && docker compose stop app     # der NPM-Dienst, der 80/443 belegt (oft "app")
cd /path/to/corvus && docker compose up -d

# Corvus für den Import mit dem Docker-Netzwerk von NPM verbinden
docker network ls | grep -i npm                # z. B. npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

Wählen Sie in *Import aus NPM* **MySQL / MariaDB** und geben Sie die Datenbankeinstellungen von NPM ein:
Host = Name des Datenbankdienstes (meist `db`), Port `3306` sowie die Werte von `DB_MYSQL_NAME`,
`DB_MYSQL_USER` und `DB_MYSQL_PASSWORD`. Die Verbindung ist schreibgeschützt, das Passwort wird nicht
gespeichert. Danach *Vorschau* → *Auswahl importieren*.

Nach Abschluss des Imports:

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # auch die NPM-Datenbank stoppen
```

#### Nach dem Import

- Prüfen Sie die importierten Hosts, entfernen Sie dann `CORVUS_IMPORT_DIR` und die beiden `/import`-Mounts
  und führen Sie erneut `docker compose up -d` aus.
- Zertifikate werden in Corvus weiter erneuert. Hosts mit einem nicht unterstützten DNS-Anbieter werden ohne
  automatische Erneuerung importiert – die Vorschau weist darauf hin.
- **Zurückwechseln:** `docker compose down` in Corvus, dann `docker compose up -d` in NPM.

## Entwicklung

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only, no OpenResty
```

| Pfad | Inhalt |
|---|---|
| `gateway/` | Datenebene: OpenResty + Lua (Regeln, Body-Prüfung, Zugriffswarteschlange) |
| `control/` | Steuerungsebene (Go): Einstellungs-DB, Konfigurationsgenerierung, Deploy/Rollback, API |
| `web/` | Admin-Konsole (React + Vite + TypeScript) |
| `deploy/` | Dockerfile, verifizierter OpenResty-Build, Compose-Dateien, Integrationstests |
| `tools/` | Lasttest- und Regel-Bundle-Signierwerkzeuge |

## Sicherheit

Bitte melden Sie Sicherheitslücken vertraulich — siehe [SECURITY.md](../../SECURITY.md).

## Lizenz

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon.
IP-zu-Land-Daten von [DB-IP](https://db-ip.com) (CC BY 4.0). Siehe [NOTICE](../../NOTICE).
