-- Lists: kind (ip | user_agent) and optional URL feeds refreshed by corvus-control.
ALTER TABLE ip_lists ADD COLUMN kind TEXT NOT NULL DEFAULT 'ip';
ALTER TABLE ip_lists ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE ip_lists ADD COLUMN refresh_s INTEGER NOT NULL DEFAULT 3600;
ALTER TABLE ip_lists ADD COLUMN fetched_at TIMESTAMP NULL;   -- last successful fetch
ALTER TABLE ip_lists ADD COLUMN checked_at TIMESTAMP NULL;   -- last fetch attempt
ALTER TABLE ip_lists ADD COLUMN fetch_error TEXT NOT NULL DEFAULT '';
ALTER TABLE ip_lists ADD COLUMN fetch_warning TEXT NOT NULL DEFAULT '';
