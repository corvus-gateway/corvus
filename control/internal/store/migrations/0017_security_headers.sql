-- Security header preset per proxy host: off | basic | strict. Existing hosts keep "off".
ALTER TABLE proxy_hosts ADD COLUMN security_headers TEXT NOT NULL DEFAULT 'off';
