-- Per stream (TCP only): accept the PROXY protocol on the incoming port (client address
-- taken from the header of trusted PROXY protocol sources) and send it to the upstream.
ALTER TABLE streams ADD COLUMN proxy_protocol_in INTEGER NOT NULL DEFAULT 0;
ALTER TABLE streams ADD COLUMN proxy_protocol_out INTEGER NOT NULL DEFAULT 0;
