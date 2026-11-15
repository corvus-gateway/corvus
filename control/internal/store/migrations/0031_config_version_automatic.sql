-- Configuration versions made automatically (fallback to an earlier configuration, safe
-- mode) when the active one could not be used. Unlike rollbacks chosen by an operator,
-- they are not kept at the next start: the stored settings are tried again.
ALTER TABLE config_versions ADD COLUMN automatic INTEGER NOT NULL DEFAULT 0;
