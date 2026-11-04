-- Redirection hosts (NPM "redirection hosts"): domains answered with a redirect to another domain.
CREATE TABLE redirection_hosts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    domains         TEXT NOT NULL,                 -- JSON array
    forward_scheme  TEXT NOT NULL DEFAULT 'auto',  -- auto ($scheme) | http | https
    forward_domain  TEXT NOT NULL,                 -- target host[:port][/path]
    http_code       INTEGER NOT NULL DEFAULT 301,
    preserve_path   INTEGER NOT NULL DEFAULT 1,
    certificate_id  INTEGER REFERENCES certificates(id) ON DELETE SET NULL,
    ssl_forced      INTEGER NOT NULL DEFAULT 0,
    http2           INTEGER NOT NULL DEFAULT 1,
    block_exploits  INTEGER NOT NULL DEFAULT 1,
    advanced_config TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);
