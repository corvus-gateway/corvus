-- Custom locations per proxy host (JSON list of store.Location: path, forward
-- scheme/host/port, optional forward path and advanced snippet). Existing hosts have none.
ALTER TABLE proxy_hosts ADD COLUMN locations TEXT NOT NULL DEFAULT '[]';
