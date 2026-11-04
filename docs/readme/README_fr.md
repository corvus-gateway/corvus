<div align="center">

[English](../../README.md) · [한국어](README_ko.md) · [简体中文](README_zh.md) · [日本語](README_ja.md) · [Русский](README_ru.md) · **Français** · [Deutsch](README_de.md) · [Español](README_es.md)

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

Corvus est une passerelle auto-hébergée placée devant vos services web. Elle est aussi simple à
exploiter que Nginx Proxy Manager — domaines, redirection et SSL gratuit en quelques clics — et elle
**surveille, protège et contrôle** en plus le trafic qui la traverse : supervision en direct, blocage
des attaques, limitation de débit et file d'accès pour absorber les pics de trafic. Tout s'exécute
sur votre propre serveur ; aucun trafic n'est envoyé vers un cloud tiers.

- [Installation rapide](#installation-rapide)
- [Installation complète](#installation-complète)
- [Captures d'écran](#captures-décran)

## Objectif du projet

Les petites équipes finissent généralement avec un reverse proxy pour le TLS, un WAF distinct qu'elles
ne règlent jamais, et aucune idée de ce qui se passe quand un lancement ou une attaque survient.
Corvus réunit tout cela dans un seul composant avec des valeurs par défaut sûres : la détection
des attaques de base fonctionne dès qu'un hôte est ajouté, sans écrire la moindre règle, et chaque
option avancée reste facultative. L'objectif est une passerelle que l'on installe en cinq minutes et
à laquelle on peut encore faire confiance le jour où le trafic est multiplié par dix.

Corvus n'est pas un fork de Nginx Proxy Manager. C'est une nouvelle implémentation (plan de données
OpenResty, plan de contrôle en Go, console React) capable d'**importer une configuration NPM existante**.

## Fonctionnalités

**Proxy et TLS**
- Hôtes proxy avec jokers (wildcards), WebSocket, HTTP/2, HTTPS forcé et extraits Nginx personnalisés
- Certificats Let's Encrypt gratuits (HTTP-01, ou DNS-01 via Cloudflare) avec renouvellement automatique, ou import des vôtres
- Véritable IP client derrière Cloudflare ou des proxys de confiance ; protection par mot de passe optionnelle par hôte
- Chaque modification est validée avec `openresty -t`, appliquée de façon atomique et peut être annulée

**Corvus Sentinel — moteur de sécurité**
- Règles de sécurité : correspondance sur IP, liste d'IP, pays, chemin, requête, méthode ou en-tête → bloquer, autoriser ou journaliser, globalement ou par hôte
- Inspection du corps des requêtes JSON, formulaire et multipart (journalisation ou blocage)
- **Correctifs virtuels** en un clic pour les CVE récentes (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Mises à jour de signatures gérées, acceptées uniquement si elles sont signées et plus récentes (pas de falsification, pas de rétrogradation)
- Bannissement automatique des IP récidivistes, avec escalade et exemptions
- Limites de débit avec mode simulation (dry-run) ; blocage des chemins d'exploitation prêt à l'emploi

**File d'accès**
- Admettre les visiteurs jusqu'à une capacité donnée et mettre les autres en file d'attente équitablement lors des pics
- Débit d'admission, politique fail-open / fail-closed, compteurs actifs/en attente en direct, libération d'urgence
- Page d'attente localisée en 8 langues

**Supervision et alertes**
- Requêtes/s en direct, codes d'état, latence p50/p95/p99, détail par hôte et par chemin
- Recherche dans le journal des requêtes et export CSV/JSON, événements de sécurité, historique persistant avec rétention et rotation des journaux
- Alertes vers Slack, Mattermost, Discord, Teams, Telegram ou webhooks signés
- `/metrics` Prometheus

**Administration**
- Rôles, MFA (TOTP + codes de récupération), authentification unique OIDC, jetons d'API
- Liste d'autorisation d'IP pour la console d'administration, journal d'audit, historique des déploiements
- SQLite par défaut ou PostgreSQL ; console d'administration en 8 langues avec thèmes clair et sombre

**Durci par défaut**
- Conteneurs non root avec système de fichiers racine en lecture seule et sans capacités Linux
- Redis protégé par un mot de passe généré et une ACL minimale
- OpenResty compilé à partir de sources dont la signature est vérifiée

## Captures d'écran

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="Tableau de bord" width="900">
</p>

| Supervision du trafic | Règles de sécurité |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="Supervision du trafic"> | <img src="../../assets/screenshots/security-rules.png" alt="Règles de sécurité"> |
| **Hôte proxy** | **Files d'accès** |
| <img src="../../assets/screenshots/proxy-host.png" alt="Hôte proxy"> | <img src="../../assets/screenshots/access-queues.png" alt="Files d'accès"> |

## Installation rapide

1. [Installez Docker](https://docs.docker.com/engine/install/) avec le plugin Compose.
2. Créez un fichier `docker-compose.yml` comme celui-ci :

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

C'est toute l'installation : la passerelle, la console d'administration et le stockage de la file
d'attente d'accès fonctionnent dans ce seul conteneur. Consultez [Installation complète](#installation-complète)
pour les options.

3. Démarrez votre stack avec

```bash
docker compose up -d
```

4. Lisez le jeton de configuration à usage unique

```bash
docker compose logs corvus | grep setup_token
```

5. Connectez-vous à la console d'administration sur `https://<your-server>:9443`, saisissez le jeton et créez
   le premier administrateur. La console utilise un certificat auto-signé tant que vous n'avez pas configuré
   le vôtre.

> [!TIP]
> Faites pointer le DNS de votre domaine vers le serveur, ajoutez un hôte proxy et demandez un certificat
> Let's Encrypt dans le même formulaire.

### Mise à jour

```bash
docker compose pull
docker compose up -d
```

Les paramètres, certificats et l'historique dans `./data` sont conservés ; les modifications de la base
de données sont appliquées automatiquement au démarrage.

## Installation complète

### Ports

| Port | Rôle |
|---|---|
| 80 / 443 | Passerelle — trafic proxifié |
| 9443 | Console d'administration et API — à n'autoriser que depuis des réseaux de confiance (pare-feu, ou *Réseau → Réseaux autorisés pour la console d'administration*) |

### Données et sauvegardes

Tout l'état se trouve dans `./data` : la base de données des paramètres, l'historique de la configuration
générée, les certificats, l'historique du trafic et `secrets/`. **Sauvegardez `secrets/`** : il contient les clés
qui chiffrent les secrets MFA et les jetons DNS dans la base de données. Le conteneur s'exécute avec
l'utilisateur non privilégié `10001` et prend la propriété de `./data` au premier démarrage.

### Installation renforcée

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml) exécute la même image avec un système
de fichiers racine en lecture seule, sans aucune capability Linux et sans élévation de privilèges. Créez
d'abord le répertoire de données pour l'utilisateur `10001` :

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### Configuration

Définissez ces variables sous `environment:` dans `docker-compose.yml`.

| Variable | Description |
|---|---|
| `CORVUS_ADMIN_HOSTS` | Noms/IP supplémentaires pour le certificat d'administration, p. ex. `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | Utiliser PostgreSQL au lieu de SQLite, p. ex. `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | Utiliser un Redis/Valkey externe pour la file d'attente d'accès (plusieurs passerelles partageant une même file) ; avec `CORVUS_REDIS_PASSWORD`. Par défaut : intégré |
| `CORVUS_IMPORT_DIR` | Répertoire contenant les volumes NPM en lecture seule pour *Importer depuis NPM* |
| `CORVUS_RULES_URL` | URL facultative d'un lot de règles gérées signé |
| `CORVUS_ADMIN_ALLOW_ANY=1` | Interrupteur de secours si vous vous êtes verrouillé avec la liste d'autorisation d'administration |

### Migrer depuis Nginx Proxy Manager

Montez les répertoires `data` et `letsencrypt` de NPM en lecture seule et indiquez-les à Corvus :

```yml
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

Ouvrez ensuite *Importer depuis NPM* dans la console. Les hôtes, certificats et listes d'accès sont repris.
Arrêtez ensuite NPM pour que les deux ne se disputent pas les ports 80/443 et les renouvellements de certificats.

## Développement

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only, no OpenResty
```

| Chemin | Contenu |
|---|---|
| `gateway/` | Plan de données : OpenResty + Lua (règles, inspection du corps, file d'accès) |
| `control/` | Plan de contrôle (Go) : base de paramètres, génération de configuration, déploiement/rollback, API |
| `web/` | Console d'administration (React + Vite + TypeScript) |
| `deploy/` | Dockerfile, build OpenResty vérifié, fichiers Compose, tests d'intégration |
| `tools/` | Outils de test de charge et de signature des lots de règles |

## Sécurité

Merci de signaler les vulnérabilités de manière privée — voir [SECURITY.md](../../SECURITY.md).

## Licence

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon.
Données IP-pays fournies par [DB-IP](https://db-ip.com) (CC BY 4.0). Voir [NOTICE](../../NOTICE).
