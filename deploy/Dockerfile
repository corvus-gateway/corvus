# Corvus (Corvus) — single image: OpenResty data plane + corvus-control (PID 1).
# Build from the repository root:  docker build -f deploy/Dockerfile -t corvus .
#
# OpenResty is compiled from verified sources (deploy/openresty/build.sh): OpenResty 1.31.8.1,
# OpenSSL 3.5.9, PCRE2 10.49, zlib 1.3.2 on Alpine 3.24 — the official image links an older
# OpenSSL. Unused nginx modules (HTTP/3, mp4, flv, dav, mail, ssi …) are not compiled in.

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS openresty
COPY deploy/openresty/build.sh /build.sh
RUN JOBS="$(nproc)" sh /build.sh

FROM node:26-alpine@sha256:c6ad9e9c1d80f6eb607c26b4b0a132ff7a2344fad1c78ea3baa2a7d4c8de6d94 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npx tsc -b && npx vite build --outDir /out/dist --emptyOutDir

FROM golang:1.27-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS control
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY control/ control/
COPY --from=web /out/dist/ control/internal/webui/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 GOAMD64=v1 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/corvus-control ./control/cmd/corvus-control

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
# Latest Alpine security updates + minimal runtime (OpenSSL/PCRE2/zlib are linked statically).
RUN apk upgrade --no-cache && apk add --no-cache libgcc ca-certificates tzdata su-exec setpriv valkey
COPY --from=openresty /usr/local/openresty /usr/local/openresty
COPY gateway/lua/   /opt/corvus/lua/
COPY gateway/redis/ /opt/corvus/redis/
COPY gateway/html/  /opt/corvus/html/
COPY --chmod=0755 deploy/entrypoint.sh /usr/local/bin/corvus-entrypoint
COPY --from=control /out/corvus-control /usr/local/bin/corvus-control
ENV PATH=/usr/local/openresty/bin:/usr/local/openresty/nginx/sbin:$PATH CORVUS_DATA_DIR=/data
# Unprivileged runtime user (H9): control plane, nginx and the embedded Valkey all run as
# uid 10001. The entrypoint starts as root only to fix the owner of a bind-mounted /data,
# then drops privileges (or runs directly under `user: "10001:10001"`).
RUN addgroup -S -g 10001 corvus && adduser -S -D -H -u 10001 -G corvus -s /sbin/nologin corvus  && mkdir -p /data /redis-auth && chown corvus:corvus /data /redis-auth
VOLUME ["/data"]
EXPOSE 80 443 9443
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s \
  CMD wget -q -O /dev/null http://127.0.0.1:9180/healthz || exit 1
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/corvus-entrypoint"]
