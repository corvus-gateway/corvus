-- "Under attack" mode per proxy host: off | on | auto (existing hosts stay off), and
-- its parameters as JSON (challenge type, difficulty, pass lifetime, binding, auto
-- thresholds; see store.UnderAttackConfig). Auto activations live in meta "under_attack".
ALTER TABLE proxy_hosts ADD COLUMN under_attack TEXT NOT NULL DEFAULT 'off';
ALTER TABLE proxy_hosts ADD COLUMN under_attack_config TEXT NOT NULL DEFAULT '{}';
