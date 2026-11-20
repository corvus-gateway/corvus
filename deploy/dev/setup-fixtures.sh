#!/bin/sh
# Prepares test fixtures for the development stack (run once in deploy/dev):
#   - pebble-ca.pem      : CA of the Pebble test ACME server (trusted by corvus via LEGO_CA_CERTIFICATES)
#   - import/npm/...     : a small Nginx Proxy Manager installation (SQLite + certificate files)
#   - fixtures/...       : HTTPS file server for list feeds (service "fixtures", trusted by corvus
#                          via CORVUS_TEST_FEED_*): a test CA, a server certificate and two IP
#                          blocklists of 1.2 million entries, with and without the private ranges
#                          the integration checks connect from
set -eu
cd "$(dirname "$0")"

PEBBLE_IMAGE=$(awk '/image: ghcr.io\/letsencrypt\/pebble/ {print $2}' docker-compose.yml)
docker pull -q "$PEBBLE_IMAGE" >/dev/null
cid=$(docker create "$PEBBLE_IMAGE")
docker cp "$cid:/test/certs/pebble.minica.pem" pebble-ca.pem
docker rm "$cid" >/dev/null

rm -rf import && mkdir -p import/npm/data/custom_ssl/npm-2 import/npm/letsencrypt/live/npm-1
mkcert() { # dir domain
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 60 -subj "/CN=$2" \
    -addext "subjectAltName=DNS:$2" -keyout "$1/privkey.pem" -out "$1/fullchain.pem" 2>/dev/null
}
mkcert import/npm/letsencrypt/live/npm-1 npm-shop.corvus.test
mkcert import/npm/data/custom_ssl/npm-2 npm-custom.corvus.test

python3 - <<'PY'
import sqlite3
db = sqlite3.connect("import/npm/data/database.sqlite")
db.executescript("""
CREATE TABLE user (id INTEGER PRIMARY KEY, email TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE certificate (id INTEGER PRIMARY KEY, owner_user_id INTEGER, provider TEXT, nice_name TEXT, domain_names TEXT, meta TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list (id INTEGER PRIMARY KEY, name TEXT, satisfy_any INTEGER, pass_auth INTEGER DEFAULT 1, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list_client (id INTEGER PRIMARY KEY, access_list_id INTEGER, address TEXT, directive TEXT);
CREATE TABLE access_list_auth (id INTEGER PRIMARY KEY, access_list_id INTEGER, username TEXT, password TEXT);
CREATE TABLE proxy_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT, forward_host TEXT, forward_port INTEGER,
  access_list_id INTEGER DEFAULT 0, certificate_id INTEGER DEFAULT 0, ssl_forced INTEGER, block_exploits INTEGER, allow_websocket_upgrade INTEGER,
  http2_support INTEGER, enabled INTEGER, advanced_config TEXT, locations TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE stream (id INTEGER PRIMARY KEY, is_deleted INTEGER DEFAULT 0);
CREATE TABLE redirection_host (id INTEGER PRIMARY KEY, is_deleted INTEGER DEFAULT 0);
INSERT INTO user VALUES (1, 'ops@corvus.test', 0);
INSERT INTO certificate VALUES (1, 1, 'letsencrypt', 'npm shop', '["npm-shop.corvus.test"]', '{}', 0);
INSERT INTO certificate VALUES (2, 1, 'other', 'npm custom', '["npm-custom.corvus.test"]', '{}', 0);
INSERT INTO certificate VALUES (3, 1, 'other', 'deleted', '["gone.corvus.test"]', '{}', 1);
INSERT INTO access_list VALUES (1, 'office', 1, 1, 0);
INSERT INTO access_list_client VALUES (1, 1, '10.99.0.0/16', 'allow');
INSERT INTO access_list_auth VALUES (1, 1, 'alice', 'alice-secret-1');
INSERT INTO proxy_host VALUES (1, '["npm-shop.corvus.test"]', 'http', 'demo', 80, 0, 1, 1, 1, 1, 1, 1, 'client_max_body_size 20m;', '[]', 0);
INSERT INTO proxy_host VALUES (2, '["npm-custom.corvus.test"]', 'http', 'demo', 80, 1, 2, 0, 0, 0, 0, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (3, '["npm-legacy.corvus.test"]', 'http', 'demo', 80, 0, 0, 0, 0, 0, 0, 1, 'proxy_pass $forward_scheme://$server:$port;', '[{"path":"/x"}]', 0);
INSERT INTO proxy_host VALUES (4, '["deleted.corvus.test"]', 'http', 'demo', 80, 0, 0, 0, 0, 0, 0, 1, '', '[]', 1);
INSERT INTO stream VALUES (1, 0);
""")
db.commit()
PY
chmod -R a+rX import

# Feed server: test CA → certificate for "fixtures"; nginx (the Corvus image's OpenResty).
rm -rf fixtures && mkdir -p fixtures/tls fixtures/www
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3650 -subj "/CN=Corvus test feeds CA" \
  -keyout fixtures/tls/ca.key -out fixtures/tls/ca.pem 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=fixtures" \
  -keyout fixtures/tls/key.pem -out fixtures/tls/req.csr 2>/dev/null
printf 'subjectAltName=DNS:fixtures\n' > fixtures/tls/ext.cnf
openssl x509 -req -in fixtures/tls/req.csr -CA fixtures/tls/ca.pem -CAkey fixtures/tls/ca.key -CAcreateserial \
  -days 3650 -extfile fixtures/tls/ext.cnf -out fixtures/tls/cert.pem 2>/dev/null
rm -f fixtures/tls/ca.key fixtures/tls/req.csr fixtures/tls/ext.cnf fixtures/tls/ca.srl
cat > fixtures/nginx.conf <<'CONF'
worker_processes 1;
pid /tmp/fixtures.pid;
error_log /dev/stderr warn;
events {}
http {
    access_log off;
    client_body_temp_path /tmp/fixtures-body;
    proxy_temp_path /tmp/fixtures-proxy;
    default_type text/plain;
    server {
        listen 8443 ssl;
        ssl_certificate /fixtures/tls/cert.pem;
        ssl_certificate_key /fixtures/tls/key.pem;
        root /fixtures/www;
    }
}
CONF
# 1.2 million public IPv4 entries (some /24s) and a few IPv6 networks; 198.51.100.7 is listed,
# 203.0.113.0/24 is not. blocklist-in.txt adds the private ranges test clients come from.
python3 - <<'PY'
import random
random.seed(252)
skip = [(10, 8), (100, 10), (127, 8), (169, 8), (172, 8), (192, 8), (203, 8), (224, 3)]
lines = ["# Corvus integration test blocklist", "198.51.100.7"]
while len(lines) < 1200002:
    a = random.randint(1, 223)
    if a in (10, 100, 127, 169, 172, 192, 203):
        continue
    b, c, d = random.randint(0, 255), random.randint(0, 255), random.randint(0, 255)
    lines.append(f"{a}.{b}.{c}.0/24" if len(lines) % 20 == 0 else f"{a}.{b}.{c}.{d}")
lines += [f"2001:db8:{i:x}::/48" for i in range(1000)]
body = "\n".join(lines) + "\n"
open("fixtures/www/blocklist-out.txt", "w").write(body)
open("fixtures/www/blocklist-in.txt", "w").write(body + "10.0.0.0/8\n172.16.0.0/12\n192.168.0.0/16\n")
PY
chmod -R a+rX fixtures
echo "fixtures ready"
