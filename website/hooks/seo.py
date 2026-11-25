"""Build hooks for search engines and AI assistants.

The docs are served under /docs/ (English) and /ko/docs/ (Korean). The product homepage at / and
/ko/ (website/landing/) carries the SoftwareApplication, SoftwareSourceCode and VideoObject nodes;
the docs pages refer to them by @id.

* Schema.org JSON-LD on every docs page: WebSite and Organization everywhere, WebPage on the docs
  home, TechArticle on the other pages, a BreadcrumbList from the homepage down, FAQPage on pages
  with `faq: true` in their front matter (each `###` heading is a question, the text up to the next
  heading is the answer).
* llms.txt (an index with one line per page) and llms-full.txt (every page as Markdown, in
  navigation order, with absolute links), see https://llmstxt.org/. They are written next to the
  homepage (site/ and site/ko/), one level above the docs.

No extra dependencies: only MkDocs' own hook events.
"""

from __future__ import annotations

import html
import json
import os
import posixpath
import re

_pages: list = []  # (page, markdown) in build order, reset per build

GITHUB = "https://github.com/corvus-gateway/corvus"
SAME_AS = [
    GITHUB,
    "https://github.com/corvus-gateway",
    "https://www.producthunt.com/products/corvus-awpg",
    "https://wikidocs.net/book/22096",
    "https://www.youtube.com/@CorvusGateway",
    "https://mastodon.social/@moonjikwang",
]
# Defined on the homepage (website/landing/build.py); docs pages only refer to it.
APP_ID = "https://corvusgateway.org/#software"


def _lang(config) -> str:
    return config.theme["language"]


def _origin(config) -> str:
    return config.extra["origin"].rstrip("/")


def _abs(config, path: str = "") -> str:
    """Absolute URL of a docs path (site_url is the docs root, e.g. https://corvusgateway.org/docs/)."""
    return config.site_url.rstrip("/") + "/" + path.lstrip("/")


def _home(config) -> str:
    """The product homepage in this language, e.g. https://corvusgateway.org/ko/."""
    return config.extra["homepage"].rstrip("/") + "/"


def _text(fragment: str) -> str:
    """Rendered HTML fragment -> plain text with collapsed whitespace."""
    fragment = re.sub(r'<a class="headerlink"[^>]*>.*?</a>', "", fragment, flags=re.S)
    fragment = re.sub(r"<[^>]+>", " ", fragment)
    return re.sub(r"\s+", " ", html.unescape(fragment)).strip()


def _strip_title(title) -> str:
    return re.sub(r"<[^>]+>", "", str(title or "")).strip()


def _title(page) -> str:
    """Front matter title, else the page's own H1, else the navigation label."""
    return page.meta.get("title") or page.meta.get("_h1") or _strip_title(page.title)


def _first_page(section):
    for child in section.children:
        if child.is_page:
            return child
        if child.is_section:
            found = _first_page(child)
            if found:
                return found
    return None


# ---------------------------------------------------------------- collect pages


def on_pre_build(config):
    _pages.clear()


def on_page_markdown(markdown, page, config, files):
    _pages.append((page, markdown))
    h1 = re.search(r"^# (.+?)[ \t]*(?:\{[^}]*\})?$", markdown, re.M)
    if h1:
        page.meta["_h1"] = h1.group(1).strip()
    return markdown


def on_page_content(content, page, config, files):
    if page.meta.get("faq"):
        page.meta["_faq"] = _faq(content)
    return content


def _faq(content: str) -> list[tuple[str, str]]:
    parts = re.split(r"(<h[1-6][^>]*>.*?</h[1-6]>)", content, flags=re.S)
    items, question, answer = [], None, []
    for part in parts:
        heading = re.match(r"<h([1-6])", part)
        if heading:
            if question and answer:
                items.append((question, _text("".join(answer))))
            question = _text(part) if heading.group(1) == "3" else None
            answer = []
        elif question is not None:
            answer.append(part)
    if question and answer:
        items.append((question, _text("".join(answer))))
    return [(q, a) for q, a in items if a]


# ---------------------------------------------------------------- JSON-LD


def _graph(page, config) -> list[dict]:
    lang = _lang(config)
    origin = _origin(config)
    home = _home(config)
    url = _abs(config, page.url)
    org = {
        "@type": "Organization",
        "@id": origin + "/#organization",
        "name": "Corvus",
        "url": origin + "/",
        "logo": origin + "/assets/logo.svg",
        "sameAs": SAME_AS,
    }
    # Same @id as the WebSite node on the homepage of this language.
    website = {
        "@type": "WebSite",
        "@id": home + "#website",
        "name": config.site_name,
        "url": home,
        "description": config.site_description,
        "inLanguage": lang,
        "publisher": {"@id": org["@id"]},
        "about": {"@id": APP_ID},
    }
    graph = [org, website]
    description = page.meta.get("description") or config.site_description
    title = config.extra["docs_title"] if page.is_homepage else _title(page)

    crumbs = [{"name": config.site_name, "item": home},
              {"name": config.extra["docs_title"], "item": _abs(config)}]
    if page.is_homepage:
        crumbs.pop()
    for ancestor in reversed(page.ancestors):
        first = _first_page(ancestor)
        if first and first is not page:
            crumbs.append({"name": _strip_title(ancestor.title), "item": _abs(config, first.url)})
    crumbs.append({"name": title, "item": url})
    graph.append({
        "@type": "BreadcrumbList",
        "itemListElement": [
            {"@type": "ListItem", "position": i + 1, "name": c["name"], "item": c["item"]}
            for i, c in enumerate(crumbs)
        ],
    })

    article = {
        "@type": page.meta.get("schema_type", "WebPage" if page.is_homepage else "TechArticle"),
        "@id": url + "#article",
        "headline": title[:110],
        "description": description,
        "url": url,
        "mainEntityOfPage": url,
        "inLanguage": lang,
        "isPartOf": {"@id": website["@id"]},
        "about": {"@type": "SoftwareApplication", "@id": APP_ID, "name": "Corvus AWPG"},
        "author": {"@id": org["@id"]},
        "publisher": {"@id": org["@id"]},
        "image": origin + "/assets/social.png",
    }
    if page.meta.get("keywords"):
        article["keywords"] = page.meta["keywords"]
    mentions = page.meta.get("mentions")
    if mentions:
        article["mentions"] = [
            {"@type": "SoftwareApplication", "name": m["name"], "url": m["url"]} for m in mentions
        ]
    graph.append(article)

    faq = page.meta.get("_faq")
    if faq:
        graph.append({
            "@type": "FAQPage",
            "@id": url + "#faq",
            "url": url,
            "inLanguage": lang,
            "isPartOf": {"@id": website["@id"]},
            "mainEntity": [
                {"@type": "Question", "name": q, "acceptedAnswer": {"@type": "Answer", "text": a}}
                for q, a in faq
            ],
        })
    return graph


def on_post_page(output, page, config):
    data = {"@context": "https://schema.org", "@graph": _graph(page, config)}
    payload = json.dumps(data, ensure_ascii=False, separators=(",", ":")).replace("</", "<\\/")
    tag = f'<script type="application/ld+json">{payload}</script>\n'
    return output.replace("</head>", tag + "</head>", 1)


# ---------------------------------------------------------------- llms.txt

_LINK = re.compile(r"(!?\[[^\]]*\]\()([^)\s]+)(\))")
_ATTR = re.compile(r"\)\{[^}\n]*\}")
_ICON = re.compile(r":(?:material|fontawesome|octicons|simple)-[\w-]+:\s*")
_FRONT = re.compile(r"\A---\n.*?\n---\n", re.S)

LLMS_INTRO = {
    "en": (
        "Corvus AWPG (Active Web Protection Gateway) is a free, open-source (Apache-2.0), self-hosted "
        "reverse proxy with a built-in web application firewall. It is as easy to run as Nginx Proxy "
        "Manager (web console, proxy hosts, free Let's Encrypt SSL) and adds the security you would "
        "otherwise bolt on: exploit blocking on by default, security rules (IP, country, path, header, "
        "User-Agent), automatic IP bans, rate limits, a browser challenge and 'under attack' mode, a "
        "waiting room (access queue) for traffic spikes, a live traffic monitor, request logs and alerts. "
        "One Docker container (OpenResty + Go), amd64 and arm64, console in 8 languages. It can import "
        "an existing Nginx Proxy Manager setup."
    ),
    "ko": (
        "Corvus AWPG(Active Web Protection Gateway)는 웹 방화벽(WAF)을 내장한 무료 오픈소스(Apache-2.0) "
        "셀프 호스팅 리버스 프록시입니다. Nginx Proxy Manager처럼 웹 콘솔에서 프록시 호스트와 무료 "
        "Let's Encrypt SSL을 설정하고, 기본으로 켜진 공격 차단, 보안 규칙(IP·국가·경로·헤더·User-Agent), "
        "자동 IP 차단, 요청 속도 제한, 브라우저 확인과 공격 대응 모드, 접속 대기열, 실시간 트래픽 관제, "
        "요청 로그와 알림을 더했습니다. Docker 컨테이너 하나(OpenResty + Go), amd64·arm64, 콘솔 8개 언어. "
        "기존 Nginx Proxy Manager 설정을 가져올 수 있습니다."
    ),
}

LLMS_FACTS = {
    "en": [
        "License: Apache-2.0, all features free, no paid edition",
        f"Source: {GITHUB}",
        "Image: ghcr.io/corvus-gateway/gateway (amd64, arm64)",
        "Install: one docker-compose.yml; ports 80/443 for traffic, 9443 for the admin console",
        "Intro video (2 min): https://youtu.be/wtd2yBssBUk",
        "Not a fit: Kubernetes ingress, configuration from Docker labels, HTTP/3, servers with no public IP (use a tunnel in front)",
    ],
    "ko": [
        "라이선스: Apache-2.0, 모든 기능 무료, 유료 버전 없음",
        f"소스: {GITHUB}",
        "이미지: ghcr.io/corvus-gateway/gateway (amd64, arm64)",
        "설치: docker-compose.yml 하나, 트래픽은 80/443, 관리 콘솔은 9443 포트",
        "소개 영상(2분, 영어): https://youtu.be/wtd2yBssBUk",
        "맞지 않는 경우: 쿠버네티스 인그레스, Docker 라벨로 설정, HTTP/3, 공인 IP가 없는 서버(앞에 터널 필요)",
    ],
}


def _absolute_links(markdown: str, page, config) -> str:
    base = posixpath.dirname(page.file.src_uri)

    def fix(m):
        target = m.group(2)
        if re.match(r"^[a-z][a-z0-9+.-]*:", target, re.I) or target.startswith("#"):
            if target.startswith("#"):
                return m.group(1) + _abs(config, page.url) + target + m.group(3)
            return m.group(0)
        path, _, anchor = target.partition("#")
        resolved = posixpath.normpath(posixpath.join(base, path)) if path else page.file.src_uri
        if resolved.endswith(".md"):
            resolved = resolved[:-3]
            resolved = "" if resolved == "index" else (resolved[:-5] if resolved.endswith("/index") else resolved) + "/"
        url = _abs(config, resolved)
        return m.group(1) + url + ("#" + anchor if anchor else "") + m.group(3)

    return _LINK.sub(fix, markdown)


def _clean(markdown: str, page, config) -> str:
    markdown = _FRONT.sub("", markdown)
    markdown = _ATTR.sub(")", markdown)
    markdown = _ICON.sub("", markdown)
    markdown = re.sub(r"</?div[^>]*>\n?", "", markdown)
    return _absolute_links(markdown, page, config).strip()


def _nav_pages(nav):
    """[(section path, page)] in navigation order."""
    out = []

    def walk(items, trail):
        for item in items:
            if item.is_page:
                out.append((trail, item))
            elif item.is_section:
                walk(item.children, trail + [_strip_title(item.title)])

    walk(nav.items, [])
    return out


_nav = {}


def on_nav(nav, config, files):
    _nav["nav"] = nav
    return nav


def on_post_build(config):
    lang = _lang(config)
    by_page = {id(p): md for p, md in _pages}
    order = _nav_pages(_nav["nav"])

    home = _home(config)
    other = [a for a in config.extra.get("alternate", []) if a.get("lang") != lang]
    lines = [f"# {config.site_name}", "", f"> {LLMS_INTRO.get(lang, LLMS_INTRO['en'])}", ""]
    lines += [f"- {fact}" for fact in LLMS_FACTS.get(lang, LLMS_FACTS["en"])]
    en = lang == "en"
    lines += ["", f"{'Homepage' if en else '홈페이지'}: {home}",
              f"{'Documentation' if en else '문서'}: {_abs(config)}",
              f"{'Full documentation in one file' if en else '문서 전체를 한 파일로'}: {home}llms-full.txt"]
    for alt in other:
        lines.append(f"{alt['name']}: {_origin(config)}{alt['home']}llms.txt")

    section = None
    for trail, page in order:
        title = _title(page)
        start = "Start" if lang == "en" else "시작"
        heading = trail[0] if trail else (start if page.is_homepage else _strip_title(page.title))
        if heading != section:
            lines += ["", f"## {heading}", ""]
            section = heading
        desc = page.meta.get("description", "")
        lines.append(f"- [{title}]({_abs(config, page.url)})" + (f": {desc}" if desc else ""))

    full = [f"# {config.site_name}", "", f"> {LLMS_INTRO.get(lang, LLMS_INTRO['en'])}", ""]
    full += [f"- {fact}" for fact in LLMS_FACTS.get(lang, LLMS_FACTS["en"])]
    for trail, page in order:
        markdown = by_page.get(id(page))
        if markdown is None:
            continue
        full += ["", "---", "", f"Source: {_abs(config, page.url)}", "", _clean(markdown, page, config)]

    # Next to the homepage (site/, site/ko/), one level above the docs (site/docs/, site/ko/docs/).
    root = os.path.dirname(os.path.normpath(config.site_dir))
    with open(os.path.join(root, "llms.txt"), "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines) + "\n")
    with open(os.path.join(root, "llms-full.txt"), "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(full) + "\n")
