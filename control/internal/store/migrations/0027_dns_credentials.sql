-- Saved DNS provider credentials for DNS-01 certificates, entered once and reused.
-- secret is the credential JSON encrypted with the data key; it is never returned by the API.
-- Per-certificate credentials of older versions (certificates.acme.credentials) are moved
-- here by the control plane at startup, because only it holds the data key.
CREATE TABLE dns_credentials (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    provider   TEXT NOT NULL,
    secret     TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

ALTER TABLE certificates ADD COLUMN dns_credential_id INTEGER REFERENCES dns_credentials(id);
