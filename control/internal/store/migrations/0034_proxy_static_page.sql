-- Static pages: a proxy host either forwards to its backend ("forward", every existing
-- host) or serves a page itself ("page": a built-in parked / coming soon / maintenance
-- page or custom HTML, as JSON store.StaticPage).
ALTER TABLE proxy_hosts ADD COLUMN content TEXT NOT NULL DEFAULT 'forward';
ALTER TABLE proxy_hosts ADD COLUMN page TEXT NOT NULL DEFAULT '{}';
