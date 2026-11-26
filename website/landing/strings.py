"""Texts of the product homepage (template.html), English only.

Values are inserted into the template as-is, so they may contain HTML. Keep facts in line with the
docs (website/en/, website/ko/).
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
    "pill_oss": "Open source",
    "pill_signed": "Signed releases · SBOM",

    # Hero
    "hero_h1": 'The reverse proxy<br> that <span class="accent">defends your sites</span>',
    "hero_lead": ("Proxy hosts and free HTTPS as easy as Nginx Proxy Manager, with attack blocking, "
                  "automatic bans and a bot challenge built in. Free and open source, in one container."),
    "hero_cta1": "Install in 5 minutes",
    "hero_cta2": "View on GitHub",

    # Feature tabs
    "tabs_label": "Features",
    "explore": "Explore",
    "tab1": "Proxy and<br>free HTTPS",
    "tab2": "Attack blocking<br>and bans",
    "tab3": "Rules and<br>bot challenge",
    "tab4": "Live traffic<br>and alerts",
    "tab5": "Waiting<br>room",
    "tab6": "Import from<br>NPM",

    "p1_h": "Proxy and HTTPS",
    "p1_sub": "Add a site with free HTTPS in a minute",
    "p1_b1": "<strong>Let's Encrypt</strong> certificates, renewed on time, wildcards via DNS",
    "p1_b2": "<strong>WebSocket, HTTP/2</strong>, custom locations and TCP/UDP streams",
    "p1_b3": "A <strong>connection check</strong> that says why a backend can't be reached",
    "p1_alt": "The proxy host form in the Corvus console: domains, forward target and WebSocket support",

    "p2_h": "Attack blocking and bans",
    "p2_sub": "Protection is on from the first host",
    "p2_b1": "<strong>Exploit paths and request bodies</strong> checked by default, no rules to write",
    "p2_b2": "<strong>Automatic bans</strong> for repeat attackers, twice as long each time",
    "p2_b3": "<strong>Rate limits</strong> per host and an <strong>under attack</strong> mode",
    "p2_alt": "The protection settings of a proxy host: rate limit, blocking known exploit paths, request body inspection and under attack mode",

    "p3_h": "Rules and bot challenge",
    "p3_sub": "Your own rules, and a bot check without a CAPTCHA",
    "p3_b1": "Block, allow, log or challenge by <strong>IP, country, path, header or User-Agent</strong>",
    "p3_b2": "A <strong>browser challenge</strong> served by Corvus itself, no third-party service",
    "p3_b3": "Try a rule in <strong>log-only mode</strong> first, or start from a template",
    "p3_alt": "The security rules list with block, allow and challenge rules and their matches in the last 24 hours",

    "p4_h": "Live traffic and alerts",
    "p4_sub": "See everything that reaches your sites",
    "p4_b1": "<strong>Live requests</strong> by status, latency percentiles and top paths",
    "p4_b2": "<strong>Request logs</strong> you can search and export",
    "p4_b3": "<strong>Alerts</strong> to Slack, Discord, Teams, Telegram, Mattermost or a webhook",
    "p4_alt": "The traffic monitor: requests by status, latency percentiles and per-host statistics",

    "p5_h": "Waiting room",
    "p5_sub": "Let in what your service can handle, queue the rest",
    "p5_b1": "<strong>Capacity and admission rate</strong> that you choose",
    "p5_b2": "Protect the <strong>whole site or one path</strong>",
    "p5_b3": "A waiting page in <strong>8 languages</strong>, emergency release in one click",
    "p5_alt": "The access queue form: name, capacity, admission rate and expiry times",

    "p6_h": "Import from NPM",
    "p6_sub": "Bring your Nginx Proxy Manager setup with you",
    "p6_b1": "<strong>Proxy hosts, redirections, streams, certificates</strong> and access lists",
    "p6_b2": "A <strong>preview</strong> lists conflicts and skipped items before anything changes",
    "p6_b3": "<strong>NPM itself is never changed</strong>, so you can go back at any time",
    "p6_alt": "The import page: choose the NPM database type and the mounted NPM data path, then preview",

    # Works with
    "works_h": "Runs where you self-host, works with what you already use",

    # Diagram
    "dg_h": "Put a shield in front of<br> everything you expose",
    "dg_p": ("Corvus sits between the internet and your apps. Visitors get HTTPS and a fast site; "
             "exploit probes, scanners and floods stop at the gateway and never reach your apps."),
    "dg_label": "How requests flow through Corvus",
    "dg_visitors": "Visitors", "dg_visitors_d": "Browsers and apps",
    "dg_bots": "Bots and scanners", "dg_bots_d": "Exploit probes, floods",
    "dg_https": "HTTPS",
    "dg_blocked": "blocked",
    "dg_allowed": "allowed",
    "dg_corvus_d": "Checks every request",
    "dg_c1": "IP bans", "dg_c2": "Security rules", "dg_c3": "WAF", "dg_c4": "Rate limits",
    "dg_c5": "Challenge", "dg_c6": "Waiting room",
    "dg_apps": "Your apps",

    # Install
    "inst_h": "Up and running<br> in five minutes",
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

    # Three things
    "k_h": "Free, self-contained<br> and built to be checked",
    "k1": "It's free and open source",
    "k1d": ("Apache-2.0, with no paid edition, no feature held back and no license key. Read the code, "
            "run it, change it."),
    "k2": "It's one container, and it stays yours",
    "k2d": ("The gateway, the console and the queue store run in one Docker container. Requests, logs "
            "and keys stay on your server: no telemetry, no cloud account, and the release check can "
            "be turned off."),
    "k3": "It's built to be checked",
    "k3d": ('Releases are signed and ship with an SBOM and build provenance '
            '(<a href="/docs/reference/hardened/">verify an image</a>). 368 integration checks run on '
            'every change. <a href="https://www.bestpractices.dev/projects/16315">OpenSSF Best '
            'Practices</a> passing badge and Baseline-1. AI-assisted development is disclosed in the '
            '<a href="https://github.com/corvus-gateway/corvus/blob/main/AI-DECLARATION.md">AI '
            'declaration</a>.'),

    # Compare
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

    # Video
    "video_h": "Watch the 2-minute intro",
    "video_p": ("Adding a site with free HTTPS, attack blocking and bans, security rules and the "
                "browser challenge, live traffic and the waiting room."),
    "video_aria": "Watch the 2-minute intro video on YouTube",
    "video_len": "YouTube · 1:43",

    # Final
    "final_h": "Start with one site.<br> It takes five minutes.",
    "final_cta2": "Read the docs",
    "ai_h": "Short on time? Ask your favourite AI about Corvus",
    "ai_prompt": ("Summarize Corvus (https://corvusgateway.org), the open-source reverse proxy with a "
                  "built-in WAF: what it does, how it compares to Nginx Proxy Manager, and when it's "
                  "the wrong choice."),

    # Footer
    "foot_tag": "Open-source reverse proxy with a built-in WAF, automatic bans, live traffic and a waiting room.",
    "foot_label": "Footer",
    "foot_product": "Product",
    "foot_community": "Community",
    "foot_project": "Project",
    "foot_install": "Install",
    "foot_security": "Security",
    "foot_discussions": "Discussions",
    "foot_license": "License",
    "foot_ai": "AI declaration",
    "foot_releases": "Release notes",
    "foot_ko": "한국어 문서",
}
