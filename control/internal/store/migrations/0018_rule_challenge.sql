-- Security rule action "challenge": mode (js | cookie), proof-of-work difficulty in
-- leading zero bits, pass cookie lifetime and binding (ip | ip_ua).
ALTER TABLE security_rules ADD COLUMN challenge_mode TEXT NOT NULL DEFAULT 'js';
ALTER TABLE security_rules ADD COLUMN challenge_difficulty INTEGER NOT NULL DEFAULT 16;
ALTER TABLE security_rules ADD COLUMN challenge_ttl_s INTEGER NOT NULL DEFAULT 86400;
ALTER TABLE security_rules ADD COLUMN challenge_bind TEXT NOT NULL DEFAULT 'ip';
