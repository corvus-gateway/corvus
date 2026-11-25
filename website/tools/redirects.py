"""Redirect stubs for the docs' former addresses.

Until the product homepage was added, the docs were the site root: /getting-started/install/ is
now /docs/getting-started/install/, /ko/security/rules/ is /ko/docs/security/rules/. For every
built docs page this writes a small page at the old address that sends the browser on, keeping
the #anchor, and tells search engines where the page lives now (canonical + noindex).

Run by website/build.sh after the MkDocs builds and the homepage. An existing file is never
overwritten (/ and /ko/ are the homepage now). Standard library only.
"""

from __future__ import annotations

import os
import sys

SITE = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))), "site")
ORIGIN = "https://corvusgateway.org"

# (docs directory inside site/, former URL prefix, language)
DOCS = [("docs", "/", "en"), (os.path.join("ko", "docs"), "/ko/", "ko")]

STUB = """<!doctype html>
<html lang="{lang}">
<head>
<meta charset="utf-8">
<title>{title}</title>
<meta name="robots" content="noindex">
<link rel="canonical" href="{origin}{new}">
<script>location.replace("{new}" + location.hash)</script>
<meta http-equiv="refresh" content="0; url={new}">
</head>
<body>
<p>{moved} <a href="{new}">{origin}{new}</a></p>
</body>
</html>
"""

TEXT = {
    "en": ("Redirecting…", "This page has moved to"),
    "ko": ("이동 중…", "이 페이지는 다음 주소로 옮겨졌습니다:"),
}


def pages(docs_dir: str):
    """URL paths (relative to the docs root, '' for the root) of every built page."""
    for root, _dirs, files in os.walk(docs_dir):
        if "index.html" in files:
            rel = os.path.relpath(root, docs_dir).replace(os.sep, "/")
            yield "" if rel == "." else rel + "/"


def main() -> int:
    written = skipped = 0
    for docs, old_prefix, lang in DOCS:
        docs_dir = os.path.join(SITE, docs)
        if not os.path.isdir(docs_dir):
            sys.exit(f"{docs_dir} is missing: run the MkDocs builds first (website/build.sh)")
        new_prefix = "/" + docs.replace(os.sep, "/") + "/"
        title, moved = TEXT[lang]
        for rel in sorted(pages(docs_dir)):
            target = os.path.join(SITE, *(old_prefix + rel).strip("/").split("/"), "index.html")
            if os.path.exists(target):  # the homepage at / and /ko/
                skipped += 1
                continue
            os.makedirs(os.path.dirname(target), exist_ok=True)
            with open(target, "w", encoding="utf-8", newline="\n") as f:
                f.write(STUB.format(lang=lang, title=title, moved=moved, origin=ORIGIN, new=new_prefix + rel))
            written += 1
    print(f"redirects: {written} stubs for former docs addresses ({skipped} skipped: already a page)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
