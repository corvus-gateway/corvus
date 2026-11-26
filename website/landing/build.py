"""Builds the product homepage and the site root around the docs.

Run by website/build.sh after both MkDocs builds (docs in site/docs/ and site/ko/docs/):

* renders template.html with the texts in strings.py into site/index.html (English only; /ko/
  forwards to the Korean docs),
  with Schema.org JSON-LD (Organization, WebSite, WebPage, SoftwareApplication,
  SoftwareSourceCode, VideoObject);
* copies landing.css, the root-only files in root/ (robots.txt, the Google site verification
  file), the brand logos, and the images and assets that used to live at the site root
  (/img/, /assets/, /ko/img/, /ko/assets/) so links from outside keep working;
* writes /sitemap.xml and /ko/sitemap.xml (the homepage plus every docs page) in place of the
  per-docs sitemaps, and puts the docs' 404 page at /404.html.

Standard library only.
"""

from __future__ import annotations

import importlib.util
import json
import os
import re
import shutil
import sys
from urllib.parse import quote
from xml.sax.saxutils import escape

HERE = os.path.dirname(os.path.abspath(__file__))
WEBSITE = os.path.dirname(HERE)
REPO = os.path.dirname(WEBSITE)
SITE = os.path.join(REPO, "site")
ORIGIN = "https://corvusgateway.org"

sys.path.insert(0, HERE)
import strings  # noqa: E402

# SAME_AS, GITHUB and APP_ID are shared with the docs' JSON-LD.
_spec = importlib.util.spec_from_file_location("seo", os.path.join(WEBSITE, "hooks", "seo.py"))
seo = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(seo)

VIDEO = {
    "@type": "VideoObject",
    "name": "Corvus: Open-Source Reverse Proxy with a Built-in WAF (Nginx Proxy Manager Alternative)",
    "description": ("A 2-minute tour of Corvus: adding a site with free HTTPS, built-in attack blocking and "
                    "automatic bans, security rules and a browser challenge, live traffic and the access queue."),
    "thumbnailUrl": "https://i.ytimg.com/vi/wtd2yBssBUk/maxresdefault.jpg",
    "uploadDate": "2026-11-23T07:05:19-08:00",
    "duration": "PT1M43S",
    "contentUrl": "https://youtu.be/wtd2yBssBUk",
    "embedUrl": "https://www.youtube.com/embed/wtd2yBssBUk",
    "inLanguage": "en",
}

# Facts used in the SoftwareApplication node. Keep them in line with the docs home (index.md).
FEATURES = {
    "en": [
        "Reverse proxy with a web console: proxy hosts, custom locations, redirections, TCP/UDP streams",
        "Free Let's Encrypt certificates with automatic renewal, wildcard certificates via DNS",
        "Built-in web application firewall: known exploit paths and request bodies checked by default",
        "Security rules on IP, country, path, header and User-Agent, with a log-only mode",
        "Automatic IP bans for repeat attackers and per-host rate limits",
        "Browser challenge and automatic 'under attack' mode, no third-party service",
        "Access queue (waiting room) for traffic spikes",
        "Live traffic monitor, request logs, visitor statistics and alerts",
        "Import from Nginx Proxy Manager",
        "Roles, MFA, SSO (OIDC), API tokens, audit log, versioned configuration with rollback",
    ],
}

LANGS = {
    # lang: (texts, path prefix, homepage URL, docs URL, site directory)
    "en": (strings.EN, "", ORIGIN + "/", ORIGIN + "/docs/", SITE),
}

# The homepage is English only; /ko/ (the Korean docs home until the docs moved) forwards to
# the Korean docs.
KO_HOME = os.path.join(SITE, "ko")
KO_DOCS = "/ko/docs/"

CHECK = ('<span class="yes"><svg class="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5"/>'
         '</svg><span class="sr">{}</span></span>')
DASH = '<span class="no" aria-hidden="true">–</span><span class="sr">{}</span>'


def jsonld(lang: str, texts: dict, home: str, docs: str, prefix: str) -> str:
    org_id = ORIGIN + "/#organization"
    app_id = seo.APP_ID
    graph = [
        {
            "@type": "Organization",
            "@id": org_id,
            "name": "Corvus",
            "url": ORIGIN + "/",
            "logo": ORIGIN + "/assets/logo.svg",
            "sameAs": seo.SAME_AS,
        },
        {
            "@type": "WebSite",
            "@id": home + "#website",
            "name": "Corvus AWPG",
            "url": home,
            "description": texts["description"],
            "inLanguage": lang,
            "publisher": {"@id": org_id},
            "about": {"@id": app_id},
        },
        {
            "@type": "WebPage",
            "@id": home + "#webpage",
            "url": home,
            "name": texts["title"],
            "description": texts["description"],
            "inLanguage": lang,
            "isPartOf": {"@id": home + "#website"},
            "about": {"@id": app_id},
            "primaryImageOfPage": ORIGIN + prefix + "/img/dashboard.png",
            "video": {"@id": ORIGIN + "/#video"},
        },
        {
            "@type": "SoftwareApplication",
            "@id": app_id,
            "name": "Corvus AWPG",
            "alternateName": ["Corvus", "Corvus Gateway", "Corvus Active Web Protection Gateway"],
            "description": texts["description"],
            "url": ORIGIN + "/",
            "applicationCategory": "SecurityApplication",
            "applicationSubCategory": "Reverse proxy and web application firewall",
            "operatingSystem": "Linux (Docker), amd64 and arm64",
            "softwareRequirements": "Docker",
            "license": "https://www.apache.org/licenses/LICENSE-2.0",
            "isAccessibleForFree": True,
            "offers": {"@type": "Offer", "price": "0", "priceCurrency": "USD"},
            "featureList": FEATURES[lang],
            "screenshot": ORIGIN + prefix + "/img/dashboard.png",
            "image": ORIGIN + "/assets/social.png",
            "installUrl": docs + "getting-started/install/",
            "downloadUrl": "https://github.com/corvus-gateway/corvus/pkgs/container/gateway",
            "releaseNotes": seo.GITHUB + "/releases",
            "inLanguage": ["en", "ko", "zh", "ja", "ru", "fr", "de", "es"],
            "author": {"@type": "Person", "name": "Jikwang Moon", "url": "https://github.com/moonjikwang"},
            "publisher": {"@id": org_id},
            "sameAs": seo.SAME_AS,
        },
        {
            "@type": "SoftwareSourceCode",
            "@id": ORIGIN + "/#source",
            "name": "Corvus AWPG",
            "codeRepository": seo.GITHUB,
            "license": "https://www.apache.org/licenses/LICENSE-2.0",
            "programmingLanguage": ["Go", "Lua", "TypeScript"],
            "runtimePlatform": ["OpenResty", "Docker"],
            "targetProduct": {"@id": app_id},
        },
        {**VIDEO, "@id": ORIGIN + "/#video", "about": {"@id": app_id}, "publisher": {"@id": org_id}},
    ]
    data = {"@context": "https://schema.org", "@graph": graph}
    return json.dumps(data, ensure_ascii=False, separators=(",", ":")).replace("</", "<\\/")


def render(lang: str) -> None:
    texts, prefix, home, docs, out_dir = LANGS[lang]
    values = dict(texts)
    values.update({
        "lang": lang,
        "canonical": home,
        "home": prefix + "/",
        "docs": prefix + "/docs",
        "img": prefix + "/img",
        "cur_en": 'aria-current="true"' if lang == "en" else "",
        "cur_ko": 'aria-current="true"' if lang == "ko" else "",
        "yes": CHECK.format(texts["yes_sr"]),
        "no": DASH.format(texts["no_sr"]),
        "jsonld": jsonld(lang, texts, home, docs, prefix),
        # The prompt for the "ask your favourite AI" links, as a URL query value.
        "ai_q": quote(texts["ai_prompt"], safe=""),
    })
    # Attribute values (title, description) must not break out of their quotes.
    for key in ("title", "description"):
        values[key] = values[key].replace("&", "&amp;").replace('"', "&quot;")

    with open(os.path.join(HERE, "template.html"), encoding="utf-8") as f:
        template = f.read()

    def fill(m):
        key = m.group(1)
        if key not in values:
            raise KeyError(f"template.html: no text for {{{{{key}}}}} ({lang})")
        return values[key]

    page = re.sub(r"\{\{(\w+)\}\}", fill, template)
    os.makedirs(out_dir, exist_ok=True)
    with open(os.path.join(out_dir, "index.html"), "w", encoding="utf-8", newline="\n") as f:
        f.write(page)


def copy_tree(src: str, dst: str) -> None:
    shutil.copytree(src, dst, dirs_exist_ok=True)


def ko_forward() -> None:
    page = ('<!doctype html><meta charset="utf-8"><title>Corvus 문서</title>'
            f'<meta http-equiv="refresh" content="0; url={KO_DOCS}">'
            f'<link rel="canonical" href="{ORIGIN}{KO_DOCS}"><meta name="robots" content="noindex">'
            f'<script>location.replace("{KO_DOCS}" + location.hash)</script>'
            f'<p><a href="{KO_DOCS}">Corvus 문서로 이동</a></p>\n')
    os.makedirs(KO_HOME, exist_ok=True)
    with open(os.path.join(KO_HOME, "index.html"), "w", encoding="utf-8", newline="\n") as f:
        f.write(page)


def sitemap(out_dir: str, home: str | None) -> None:
    """The homepage (if any) + every docs page of one language, replacing the docs' own sitemap."""
    docs_dir = os.path.join(out_dir, "docs")
    if not os.path.exists(os.path.join(docs_dir, "sitemap.xml")):
        return  # already merged by an earlier run on the same build
    with open(os.path.join(docs_dir, "sitemap.xml"), encoding="utf-8") as f:
        locs = re.findall(r"<loc>([^<]+)</loc>", f.read())
    urls = ([home] if home else []) + [u for u in locs if u != home]
    body = "".join(f"  <url><loc>{escape(u)}</loc></url>\n" for u in urls)
    with open(os.path.join(out_dir, "sitemap.xml"), "w", encoding="utf-8", newline="\n") as f:
        f.write('<?xml version="1.0" encoding="UTF-8"?>\n'
                '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' + body + "</urlset>\n")
    for name in ("sitemap.xml", "sitemap.xml.gz"):
        path = os.path.join(docs_dir, name)
        if os.path.exists(path):
            os.remove(path)


def main() -> None:
    if not os.path.isfile(os.path.join(SITE, "docs", "index.html")):
        sys.exit("site/docs/ is missing: run the MkDocs builds first (website/build.sh)")
    for lang in LANGS:
        render(lang)
        sitemap(LANGS[lang][4], LANGS[lang][2])
    ko_forward()
    sitemap(KO_HOME, None)
    shutil.copy2(os.path.join(HERE, "landing.css"), os.path.join(SITE, "landing.css"))
    copy_tree(os.path.join(HERE, "root"), SITE)
    shutil.copytree(os.path.join(REPO, "assets", "brand"), os.path.join(SITE, "assets", "brand"),
                    dirs_exist_ok=True, ignore=shutil.ignore_patterns("*.png"))
    # Former root paths (/img/…, /assets/…), still linked from outside; the homepage uses them too.
    copy_tree(os.path.join(WEBSITE, "en", "img"), os.path.join(SITE, "img"))
    copy_tree(os.path.join(WEBSITE, "en", "assets"), os.path.join(SITE, "assets"))
    copy_tree(os.path.join(WEBSITE, "ko", "img"), os.path.join(SITE, "ko", "img"))
    copy_tree(os.path.join(WEBSITE, "ko", "assets"), os.path.join(SITE, "ko", "assets"))
    # GitHub Pages serves /404.html for every missing path; the docs' one has the search box.
    shutil.copy2(os.path.join(SITE, "docs", "404.html"), os.path.join(SITE, "404.html"))
    print("landing: site/index.html, /ko/ forward, sitemaps and root files written")


if __name__ == "__main__":
    main()
