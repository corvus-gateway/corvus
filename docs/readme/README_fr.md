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

<p align="center"><img src="../../assets/demo/corvus-demo.gif" alt="Corvus demo" width="900"></p>

- [Installation rapide](#installation-rapide)
- [Documentation](https://corvusgateway.org/)
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
- Notes sur les hôtes et les streams, pour se souvenir à quoi sert chacun (avec recherche)
- Emplacements personnalisés : des chemins (ex. `/api`) vers leur propre upstream, avec les protections de l'hôte
- Hôtes de redirection : renvoyer des domaines vers un autre domaine (301/302/303/307/308, en conservant éventuellement le chemin), avec le même TLS et la même protection
- Certificats Let's Encrypt gratuits (HTTP-01, ou DNS-01 via Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap, avec des identifiants de fournisseur enregistrés et réutilisables) avec renouvellement automatique, ou import des vôtres (remplaçables sur place lors du renouvellement)
- Flux (streams) : rediriger des ports TCP/UDP (bases de données, SSH, MQTT, serveurs de jeu) vers un autre hôte, avec terminaison TLS optionnelle pour TCP
- Les noms d’hôte inconnus reçoivent au choix une page 404, une connexion fermée ou une redirection
- Véritable IP client derrière Cloudflare ou des proxys de confiance ; protection par mot de passe optionnelle par hôte
- Préréglages d'en-têtes de sécurité par hôte (Basique / Strict : nosniff, politiques de frame et de referrer, CSP en mode rapport seul, attributs de cookie HttpOnly/SameSite/Secure), sans jamais écraser ce que l'application envoie déjà
- Compression gzip des réponses texte (activée par défaut), limite de taille d'envoi et délai d'attente de l'amont par hôte
- Chaque modification est validée avec `openresty -t`, appliquée de façon atomique et peut être annulée
- La console signale une nouvelle version de Corvus, avec un lien vers ses notes de version (désactivable)

**Corvus Sentinel — moteur de sécurité**
- Règles de sécurité : correspondance sur IP, liste d'IP, pays, chemin, requête, méthode, en-tête ou liste de User-Agent → bloquer, autoriser, journaliser ou soumettre à un défi, globalement ou par hôte
- Listes d'IP et de User-Agent tenues à jour depuis une URL (p. ex. nœuds de sortie Tor, listes de bots malveillants ; préréglages inclus), actualisées en arrière-plan et redéployées uniquement lorsqu'elles changent
- Défi navigateur sans service tiers : une preuve de travail JavaScript (difficulté réglable) ou une simple vérification par cookie, avec un cookie de passage signé et lié à l'IP
- Mode « sous attaque » par hôte : chaque visiteur doit relever le défi — activé à la main, ou automatiquement tant que le nombre de requêtes par seconde ou la part de requêtes bloquées dépasse un seuil (avec des alertes à l'activation et à la désactivation)
- Inspection du corps des requêtes JSON, formulaire et multipart (journalisation ou blocage)
- **Correctifs virtuels** en un clic pour les CVE récentes (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Mises à jour de signatures gérées, acceptées uniquement si elles sont signées et plus récentes (pas de falsification, pas de rétrogradation)
- Bannissement automatique des IP récidivistes, avec escalade et exemptions ; bannissement immédiat optionnel en cas de mauvais comportement (trop de réponses 4xx par IP et par site)
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
d'attente d'accès fonctionnent dans ce seul conteneur. Consultez la [documentation](https://corvusgateway.org/)
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

## Documentation

La configuration, l'installation renforcée, les hôtes de destination, les flux, les sauvegardes, la migration
depuis Nginx Proxy Manager, les règles de sécurité, la file d'attente d'accès et le dépannage sont décrits dans la
**[documentation](https://corvusgateway.org/)** (en anglais et en coréen).

## Performances

Mesuré avec le [workflow de benchmark](../benchmark.md) sur un runner `ubuntu-24.04` hébergé par GitHub
(4 vCPU AMD EPYC 9V45) : Corvus limité à **2 vCPU**, wrk et le backend sur les deux autres, 64 connexions
keep-alive, `GET /` avec une réponse de 3 octets.

| Scénario | req/s | p50 | p99 | Mémoire |
|---|---:|---:|---:|---:|
| nginx 1.28 en reverse proxy (référence) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus, hôte par défaut | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus, 30 règles + liste de 1,000 CIDR + blocage des exploits + en-têtes + limite de débit | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus, HTTPS (ECDSA P-256), keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus, HTTPS, nouvelle poignée de main TLS à chaque requête | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus, file d'accès (visiteurs admis) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus, règle de défi (visiteurs munis d'un passe) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

Sur le traitement de base des requêtes, Corvus atteint environ les trois quarts du débit de nginx seul
(pools de connexions keep-alive vers l'upstream, journal d'accès regroupé par worker) ; l'ensemble complet
de protections — 30 règles avec une liste de 1,000 CIDR, blocage des exploits, en-têtes de sécurité et limite
de débit — coûte environ 28 % de plus. **Mise en garde :**
il s'agit d'un runner CI partagé, avec le générateur de charge sur la même machine et des réponses minuscules —
les chiffres sont prudents et servent à comparer des scénarios et des versions, pas à garantir une capacité.
La mémoire inclut le plan de contrôle et le Valkey intégré.

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
