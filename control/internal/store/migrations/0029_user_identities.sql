-- Single sign-on identities: the identity provider account (issuer + subject) a console
-- user signs in with. One subject per user and issuer; a subject belongs to one user.
-- Existing links are filled in from earlier SSO sign-ins (see backfillIdentities).
CREATE TABLE user_identities (
    issuer     TEXT NOT NULL,
    subject    TEXT NOT NULL,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (issuer, subject),
    UNIQUE (user_id, issuer)
);
