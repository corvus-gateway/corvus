#!/bin/sh
# Builds the English site into site/ and the Korean site into site/ko/.
set -e
cd "$(dirname "$0")"
mkdocs build --strict -f mkdocs.yml
mkdocs build --strict -f mkdocs.ko.yml
