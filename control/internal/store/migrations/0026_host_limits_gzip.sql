-- Per proxy host: maximum upload size in MB (NULL = global 100 MB, 0 = unlimited),
-- upstream read/send timeout in seconds (NULL = nginx default 60 s; WebSocket hosts 1 h)
-- and gzip compression of text responses (on for new and existing hosts).
ALTER TABLE proxy_hosts ADD COLUMN max_upload_mb INTEGER;
ALTER TABLE proxy_hosts ADD COLUMN upstream_timeout_s INTEGER;
ALTER TABLE proxy_hosts ADD COLUMN gzip INTEGER NOT NULL DEFAULT 1;
