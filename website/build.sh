#!/bin/sh
# Builds the whole site into site/:
#   /          product homepage (landing/), Korean at /ko/
#   /docs/     English docs (mkdocs.yml), Korean at /ko/docs/ (mkdocs.ko.yml)
# plus redirect stubs at the docs' former addresses (/getting-started/install/ → /docs/…).
set -e
cd "$(dirname "$0")"
rm -rf ../site
mkdocs build --strict -f mkdocs.yml
mkdocs build --strict -f mkdocs.ko.yml
python landing/build.py
python tools/redirects.py
