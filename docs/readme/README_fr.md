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
<a href="https://www.bestpractices.dev/projects/16315/baseline-1"><img src="https://www.bestpractices.dev/projects/16315/baseline"></a>

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
- Certificats Let's Encrypt gratuits (HTTP-01, ou DNS-01 via Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap) avec renouvellement automatique, ou import des vôtres
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
| `CORVUS_REDIS_PORT` | Port du stockage intégré de la file d'attente d'accès sur 127.0.0.1 (par défaut `6379`) ; à modifier avec `network_mode: host` si l'hôte utilise déjà 6379 |
| `CORVUS_RESOLVER` | Serveur DNS pour les hôtes de destination. Par défaut : le DNS de Docker, ou le premier `nameserver` de `/etc/resolv.conf` en réseau hôte |
| `CORVUS_LOOPBACK=container` | Faire pointer les hôtes de destination `127.0.0.1`/`localhost` vers le conteneur lui-même plutôt que vers l'hôte Docker |

### Hôtes de destination

Un hôte proxy peut transférer vers :

- **un autre conteneur** par son nom, p. ex. `xoshop-web` port `80` — Corvus doit partager un réseau Docker
  avec lui (ajoutez ce réseau au service `corvus`) ;
- **une IP privée ou publique**, p. ex. `10.3.2.39` port `8084` ;
- **un service sur l'hôte Docker lui-même** via `127.0.0.1` ou `localhost`, comme dans NPM. Dans un
  conteneur, ces adresses désignent l'hôte Docker : `host.docker.internal` n'est donc pas nécessaire. Le service
  doit écouter sur une adresse joignable depuis le conteneur (`0.0.0.0` ou le pont Docker, pas seulement
  `127.0.0.1`), et le pare-feu de l'hôte doit autoriser les connexions provenant des réseaux Docker.

Pour atteindre des services qui n'écoutent que sur `127.0.0.1` de l'hôte, ou pour conserver tel quel un
déploiement NPM qui utilisait `network_mode: host`, exécutez Corvus en réseau hôte : remplacez `ports:` et
`networks:` par `network_mode: host` (et définissez `CORVUS_REDIS_PORT` si le port 6379 est déjà pris sur l'hôte).

### Migrer depuis Nginx Proxy Manager

*Importer depuis NPM* dans la console copie les **hôtes proxy, certificats** (Let's Encrypt et
personnalisés, y compris les jetons DNS Cloudflare) et **listes d'accès** (utilisateurs et règles IP)
d'une installation NPM existante. Seuls les fichiers et la base de données de NPM sont lus — NPM lui-même
n'est pas modifié, vous pouvez donc revenir en arrière à tout moment.

Non importés : les streams (TCP/UDP), les hôtes de redirection, les hôtes 404, les emplacements
personnalisés et la configuration avancée qui utilise des variables propres à NPM. L'aperçu liste tout ce
qui est ignoré.

**Vérifiez quelle base de données votre NPM utilise.** Ouvrez le `docker-compose.yml` de NPM :

- aucune variable `DB_MYSQL_*` / `DB_POSTGRES_*`, et `data/database.sqlite` existe → **SQLite** (par défaut dans NPM)
- `DB_MYSQL_HOST`, `DB_MYSQL_USER`, … et généralement un service `db` séparé → **MySQL / MariaDB**
- `DB_POSTGRES_*` → PostgreSQL n'est pas pris en charge par l'import ; recréez les hôtes dans la console

Dans les deux cas, les répertoires `data` et `letsencrypt` de NPM doivent être montés en lecture seule dans
Corvus (les fichiers de certificats s'y trouvent) :

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

Utilisez les chemins hôtes des volumes `./data` et `./letsencrypt` de NPM. NPM et Corvus ont tous deux
besoin des ports 80/443 et ne peuvent donc pas tourner en même temps.

#### NPM avec SQLite (par défaut)

```bash
cd /path/to/npm && docker compose down        # arrêter NPM (libère 80/443)
cd /path/to/corvus && docker compose up -d     # démarrer Corvus avec les montages ci-dessus
```

Dans la console, ouvrez *Importer depuis NPM*, conservez **SQLite**, cliquez sur *Aperçu*, choisissez ce
qu'il faut importer et cliquez sur *Importer la sélection*. Si l'aperçu signale `permission denied`, rendez
les fichiers lisibles pour Corvus (il tourne sous l'uid 10001) : `sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`.

#### NPM avec MySQL / MariaDB

Corvus doit pouvoir joindre la base de données de NPM pendant l'import : arrêtez donc uniquement le service
proxy de NPM et laissez sa base de données tourner :

```bash
cd /path/to/npm && docker compose stop app     # le service NPM qui occupe 80/443 (souvent "app")
cd /path/to/corvus && docker compose up -d

# connecter Corvus au réseau Docker de NPM pour l'import
docker network ls | grep -i npm                # p. ex. npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

Dans *Importer depuis NPM*, choisissez **MySQL / MariaDB** et saisissez les paramètres de la base de
données de NPM : hôte = nom du service de base de données (généralement `db`), port `3306`, ainsi que les
valeurs de `DB_MYSQL_NAME`, `DB_MYSQL_USER` et `DB_MYSQL_PASSWORD`. La connexion est en lecture seule et le
mot de passe n'est pas enregistré. Puis *Aperçu* → *Importer la sélection*.

Une fois l'import terminé :

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # arrêter aussi la base de données de NPM
```

#### Après l'import

- Vérifiez les hôtes importés, puis supprimez `CORVUS_IMPORT_DIR` et les deux montages `/import` et
  relancez `docker compose up -d`.
- Les certificats continuent d'être renouvelés dans Corvus. Les hôtes qui utilisaient un fournisseur DNS non
  pris en charge sont importés sans renouvellement automatique — l'aperçu le signale.
- **Revenir en arrière :** `docker compose down` dans Corvus, puis `docker compose up -d` dans NPM.

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
