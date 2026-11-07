-- Free-text notes on proxy hosts, redirection hosts and streams (shown in the console,
-- not used by the gateway configuration).
ALTER TABLE proxy_hosts ADD COLUMN notes TEXT NOT NULL DEFAULT '';
ALTER TABLE redirection_hosts ADD COLUMN notes TEXT NOT NULL DEFAULT '';
ALTER TABLE streams ADD COLUMN notes TEXT NOT NULL DEFAULT '';
