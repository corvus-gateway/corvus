-- Large IP lists (URL feeds) are kept outside the database: their merged ranges live in
-- a content-addressed file <data>/lists/<data_hash>.bin (internal/ipset) and the row only
-- records the hash and counts. Manually edited lists keep their entries in "items".
ALTER TABLE ip_lists ADD COLUMN data_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE ip_lists ADD COLUMN item_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ip_lists ADD COLUMN range_count INTEGER NOT NULL DEFAULT 0;
