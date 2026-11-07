-- Streams (NPM "streams"): TCP/UDP port forwarding.
CREATE TABLE streams (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    incoming_port   INTEGER NOT NULL,
    tcp_forwarding  INTEGER NOT NULL DEFAULT 1,
    udp_forwarding  INTEGER NOT NULL DEFAULT 0,
    forward_host    TEXT NOT NULL,
    forward_port    INTEGER NOT NULL,
    certificate_id  INTEGER REFERENCES certificates(id) ON DELETE SET NULL, -- TLS termination (TCP only)
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);
