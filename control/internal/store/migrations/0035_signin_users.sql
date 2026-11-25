-- Sign-in users: a login page in front of proxy hosts (#294). People who sign in to the
-- sites behind Corvus, separate from the users of the admin console.

CREATE TABLE signin_users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT NOT NULL UNIQUE,      -- stored lower-case
    password_hash   TEXT NOT NULL,             -- bcrypt
    totp_required   INTEGER NOT NULL DEFAULT 0,
    totp_secret     TEXT NOT NULL DEFAULT '',  -- encrypted with the data key; '' = not enrolled yet
    session_version INTEGER NOT NULL DEFAULT 1, -- bumped to end all sessions of the user
    enabled         INTEGER NOT NULL DEFAULT 1,
    note            TEXT NOT NULL DEFAULT '',
    last_login_at   TIMESTAMP,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);

CREATE TABLE signin_groups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE signin_group_members (
    group_id INTEGER NOT NULL REFERENCES signin_groups(id) ON DELETE CASCADE,
    user_id  INTEGER NOT NULL REFERENCES signin_users(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

-- Per-host login page settings, JSON store.HostLogin ('{}' = no login page).
ALTER TABLE proxy_hosts ADD COLUMN login TEXT NOT NULL DEFAULT '{}';
