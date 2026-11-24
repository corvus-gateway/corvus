# Temporary OpenResty patches

`deploy/Dockerfile` copies this directory to `/patches/` in the build stage. It is empty on purpose
(apart from this file): OpenResty 1.31.9.1 bundles nginx 1.31.9, so the nginx 1.31.9 security patch
that was applied to the bundled nginx 1.31.8 is no longer needed.

When an nginx security release ships before OpenResty bundles it:

1. Put the `src/` part of `git diff release-<bundled> release-<fixed>` from
   https://github.com/nginx/nginx here (without `src/core/nginx.h`, so the version string stays).
2. Apply it in `deploy/openresty/build.sh` right after `cd "openresty-${OPENRESTY_VERSION}"`:
   `patch -d bundle/nginx-<bundled> -p1 --forward --no-backup-if-mismatch < /patches/<file>.patch`
   (the build fails if it no longer applies).
3. The daily security workflow lists `*.patch` files here in its upstream-update issue, as a
   reminder to remove them once a new OpenResty release includes the fixes.
