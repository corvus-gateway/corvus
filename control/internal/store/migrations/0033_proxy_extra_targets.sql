-- Backend pools: additional forward targets of a proxy host (JSON list of store.Target:
-- forward host + port), served round-robin together with forward_host/forward_port.
-- Existing hosts have none (a single target, as before).
ALTER TABLE proxy_hosts ADD COLUMN extra_targets TEXT NOT NULL DEFAULT '[]';
