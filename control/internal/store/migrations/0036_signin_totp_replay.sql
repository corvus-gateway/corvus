-- Sign-in users (#294): remember the last accepted TOTP time step, so a code cannot be used
-- twice (same as users.mfa_last_step for the console).
ALTER TABLE signin_users ADD COLUMN mfa_last_step INTEGER NOT NULL DEFAULT 0;
