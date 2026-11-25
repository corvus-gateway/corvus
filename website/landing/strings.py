"""Texts of the product homepage (template.html), English only.

Values are inserted into the template as-is, so they may contain HTML. Keep facts in line with the
docs (website/en/, website/ko/), and keep both languages in step.
"""

EN = {
    "title": "Corvus AWPG: open-source reverse proxy with a built-in WAF",
    "description": ("Free, open-source and self-hosted. Proxy hosts and free HTTPS as easy as Nginx Proxy "
                    "Manager, plus attack blocking, automatic bans, a bot challenge, live traffic and a "
                    "waiting room. One Docker container."),
    "og_locale": "en_US",
    "skip": "Skip to content",
    "home_label": "home",
    "nav_label": "Main",
    "menu": "Menu",
    "nav_features": "Features",
    "nav_compare": "Compare",
    "nav_docs": "Docs",
    "nav_cta": "Get started",

    "hero_pill": "Reverse proxy with a built-in WAF",
    "hero_h1": 'The reverse proxy<br class="br-lg"> that <span class="grad">defends your sites</span>',
    "hero_lead": ("Free, open-source and self-hosted. Proxy hosts and free HTTPS as easy as Nginx Proxy "
                  "Manager, plus attack blocking, automatic bans, a bot challenge, live traffic and a "
                  "waiting room. One Docker container."),
    "hero_cta1": "Install in 5 minutes",
    "hero_cta2": "View on GitHub",
    "trust_free": "No paid edition",
    "trust_langs": "8 languages",
    "alt_dashboard": ("The Corvus console dashboard: requests per second, error rate, latency, blocked "
                      "requests, live traffic chart and access queues"),

    "ov_eyebrow": "One console",
    "ov_h2": "Everything a public server needs, without the glue",
    "ov_sub": ("A reverse proxy for TLS, a WAF, fail2ban, a log viewer and a queue for launch day: "
               "Corvus is all of them, set up with safe defaults."),
    "t1": "Proxy hosts", "t1d": "Domains, locations, redirects, TCP/UDP streams",
    "t2": "Free HTTPS", "t2d": "Let's Encrypt with auto-renewal and wildcards",
    "t3": "Built-in WAF", "t3d": "Exploit paths and request bodies, on by default",
    "t4": "Automatic bans", "t4d": "Repeat attackers locked out, for longer each time",
    "t5": "Security rules", "t5d": "IP, country, path, header and User-Agent",
    "t6": "Waiting room", "t6d": "Admit up to your capacity, queue the rest",
    "t7": "Live traffic", "t7d": "Requests, errors, latency and request logs",
    "t8": "Alerts", "t8d": "Slack, Discord, Teams, Telegram and webhooks",

    "features_label": "Features",
    "alt_hosts": "The proxy hosts list in the Corvus console",
    "alt_check": "The proxy host form with a failed connection check explaining that the port refused the connection",
    "alt_bans": "Automatic ban settings: detections, time window, first and longest ban, exempt networks",
    "alt_rules": "The security rules list with block, allow and challenge rules and their matches in the last 24 hours",
    "alt_traffic": "The traffic monitor: requests by status, latency percentiles and per-host statistics",
    "alt_queues": "The access queues page with one active queue at 3% of its capacity",
    "alt_queue_form": "The access queue form: capacity, admission rate and expiry times",
    "alt_pages": "The block and error pages editor with a live preview",
    "alt_page_form": "The proxy host form set to serve a maintenance page with status 503",
    "f1_tag": "Proxy & HTTPS",
    "f1_h": "Add a site with free HTTPS in a minute",
    "f1_p": ("Enter the domain and where the app runs, then save. Corvus gets a Let's Encrypt "
             "certificate, renews it on time and deploys the change right away."),
    "f1_b1": "Wildcards, WebSocket, HTTP/2 and custom locations",
    "f1_b2": "A connection check that says why a backend can't be reached",
    "f1_b3": "Several backends per host, round-robin with failover",
    "f1_more": "Your first proxy host",

    "f2_tag": "Protection",
    "f2_h": "Protection is on from the first host",
    "f2_p": ("Known exploit paths and request bodies are checked the moment a host is added, without "
             "writing a single rule. IPs that keep attacking are banned automatically."),
    "f2_b1": "Ban durations that double for repeat offenders",
    "f2_b2": "Rate limits per host, exempt networks for your office",
    "f2_b3": "Under attack mode that challenges every visitor of a host",
    "f2_more": "How protection works",

    "f3_tag": "Rules & challenge",
    "f3_h": "Your own rules, and a bot check without a CAPTCHA",
    "f3_p": ("Block, allow, log or challenge requests by IP, country, path, header or User-Agent. The "
             "browser challenge is served by Corvus itself: no CAPTCHA provider, no third-party "
             "service."),
    "f3_b1": "Start from templates, try a rule in log-only mode first",
    "f3_b2": "IP and User-Agent lists, including blocklists with millions of entries",
    "f3_b3": "Country blocking with a monthly updated database",
    "f3_more": "Security rules",

    "f4_tag": "Monitoring",
    "f4_h": "See everything that reaches your sites",
    "f4_p": ("Live requests by status, latency percentiles, blocked requests and top paths for every "
             "host. Search the request logs, follow security events and get an alert when something "
             "is off."),
    "f4_b1": "Request logs you can search and export",
    "f4_b2": "Visitor statistics with referrers and UTM values",
    "f4_b3": "Alerts to Slack, Discord, Teams, Telegram and more",
    "f4_more": "Traffic monitor",

    "f5_tag": "Access queue",
    "f5_h": "A waiting room for launch day",
    "f5_p": ("Let in only as many visitors as your service can handle and queue the rest in order. "
             "Waiting visitors see their position on a page that refreshes by itself."),
    "f5_b1": "Capacity and admission rate you choose",
    "f5_b2": "Protect the whole site or only one path",
    "f5_b3": "Waiting page in 8 languages, emergency release in one click",
    "f5_more": "Access queue",

    "f6_tag": "Pages",
    "f6_h": "Serve a page without a backend",
    "f6_p": ("Coming soon, maintenance, a parked domain or your own HTML, answered by the gateway "
             "itself. Security rules, HTTPS and statistics still apply."),
    "f6_b1": "Ready-made templates in the visitor's language",
    "f6_b2": "The right status code, such as 503 for maintenance",
    "f6_b3": "Your own block and error pages, with a live preview",
    "f6_more": "Pages without a backend",

    "npm_tag": "Coming from NPM?",
    "npm_h": "Bring your Nginx Proxy Manager setup with you",
    "npm_p": ("Corvus reads an NPM data folder and imports proxy hosts, redirection hosts, streams, "
              "certificates and access lists. NPM itself is never changed, so you can go back at any "
              "time."),
    "npm_cta1": "Migration guide",
    "npm_cta2": "Corvus vs NPM",
    "npm_s1": "Mount the NPM data folder", "npm_s1d": "Read-only, into the Corvus container",
    "npm_s2": "Review the preview", "npm_s2d": "Conflicts and skipped items are listed",
    "npm_s3": "Pick what to import", "npm_s3d": "Hosts keep their certificates and access lists",

    "video_eyebrow": "Video",
    "video_h": "Watch the 2-minute intro",
    "video_p": ("Adding a site with free HTTPS, attack blocking and bans, security rules and the "
                "browser challenge, live traffic and the waiting room."),
    "video_aria": "Watch the 2-minute intro video on YouTube",
    "video_title": "Corvus in 2 minutes",
    "video_len": "YouTube · 1:43",

    "inst_eyebrow": "Install",
    "inst_h": "Up and running in five minutes",
    "inst_p": ("One container holds the gateway, the admin console and the queue store. All you need "
               "is a Linux server with Docker, amd64 or arm64."),
    "inst_s1": "Save the <code>docker-compose.yml</code> and run <code>docker compose up -d</code>.",
    "inst_s2": "Read the one-time setup token from the log.",
    "inst_s3": "Open <code>https://your-server:9443</code> and create the administrator.",
    "inst_cta": "Read the install guide",
    "inst_nas": "On a NAS? TrueNAS and Unraid app catalogs",
    "code_c1": "your sites",
    "code_c2": "admin console",
    "code_term": "Terminal",

    "tr_eyebrow": "Built in the open",
    "tr_h": "Software you can check, not just trust",
    "tr_sub": "A security tool should show its work. Here is how Corvus is built and shipped.",
    "tr1": "Signed releases",
    "tr1d": ("Images are signed with Sigstore cosign and ship with an SBOM and SLSA build provenance. "
             '<a href="/docs/reference/hardened/">Verify an image</a>.'),
    "tr2": "368 integration checks",
    "tr2d": ("Every pull request builds the image and runs 368 checks against a real gateway, plus "
             "Go, Lua and TypeScript tests and an upgrade test."),
    "tr3": "OpenSSF Best Practices",
    "tr3d": ('<a href="https://www.bestpractices.dev/projects/16315">OpenSSF Best Practices</a> '
             "passing badge and Baseline-1, scored by OpenSSF Scorecard, CodeQL on every pull request."),
    "tr4": "Nothing leaves your server",
    "tr4d": ("Requests, logs and keys stay with you. No telemetry and no cloud account; the release "
             "check can be turned off."),
    "tr5": "AI-assisted, disclosed",
    "tr5d": ("Built with an AI coding assistant; the maintainer reviews every change. Where and how "
             'much: <a href="https://github.com/corvus-gateway/corvus/blob/main/AI-DECLARATION.md">AI '
             "declaration</a>."),
    "tr6": "Free, all of it",
    "tr6d": "Apache-2.0. No paid edition, no feature held back, no license key.",

    "cmp_eyebrow": "Compare",
    "cmp_h": "How Corvus compares",
    "cmp_sub": ("Good reverse proxies make different trade-offs. Here is where Corvus is different, "
                "and where the others fit better."),
    "cmp_caption": "Corvus compared with Nginx Proxy Manager, Caddy and Traefik",
    "cmp_feature": "Feature",
    "cmp_r1": "Managed in a web console",
    "cmp_r2": "WAF (attack signatures)",
    "cmp_r3": "Automatic IP bans",
    "cmp_r4": "Bot challenge without a CAPTCHA service",
    "cmp_r5": "Waiting room for traffic spikes",
    "cmp_r6": "Every feature free",
    "cmp_r7": "Configuration from Docker labels",
    "cmp_readonly": "Read-only dashboard",
    "cmp_plugin": "Plugin",
    "cmp_paid_or_plugin": "Paid or plugin",
    "cmp_hub": "Traefik Hub is paid",
    "cmp_note": "Checked against each project's documentation in November 2026.",
    "cmp_more": "Full comparison, including when Corvus is the wrong choice",
    "yes_sr": "Yes",
    "no_sr": "No",

    "final_h": "Put a shield in front of your apps today",
    "final_p": "One container, five minutes, and every feature included.",
    "final_cta2": "Read the docs",

    "foot_tag": "Open-source reverse proxy with a built-in WAF, automatic bans, live traffic and a waiting room.",
    "foot_label": "Footer",
    "foot_product": "Product",
    "foot_releases": "Releases",
    "foot_docs_home": "Documentation",
    "foot_install": "Install",
    "foot_migrate": "Migrate from NPM",
    "foot_security": "Security",
    "foot_community": "Community",
    "foot_discussions": "Discussions",
}
