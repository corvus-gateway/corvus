#!/bin/sh
# Builds OpenResty from verified sources into /usr/local/openresty (run inside alpine:3.24).
# Why not the official image: as of 2026-11-03 `openresty:1.31.8.1-alpine` links OpenSSL 3.5.8
# (missing the 2026-09-29 OpenSSL fixes) and the `-alpine-apk` variant is based on EOL Alpine 3.18.
#
# Supply chain: every tarball is checked against a pinned SHA-256 (values verified against
# upstream: OpenResty PGP signature by Yichun Zhang 2545 1EB0 8846 0026 195B D62C B550 E09E A0E9 8066,
# OpenSSL's published .sha256, PCRE2 GitHub release digest, zlib.net SHA-256).
#
# Attack surface: only the modules Corvus needs. HTTP/3 (QUIC), mp4, flv, dav, mail, image_filter,
# xslt, geoip, ssi-related extras are NOT compiled in.
set -eu

OPENRESTY_VERSION=1.31.8.1
OPENRESTY_SHA256=b569c63b420f80a4ac8466f2deffe73ed93ab760026532e039c3658409a997f4
OPENSSL_VERSION=3.5.9
OPENSSL_SHA256=603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a
PCRE2_VERSION=10.49
PCRE2_SHA256=929f0b20e62879252a15886b06c89f1edef61a363cbd5826fb041080a5e557ae
ZLIB_VERSION=1.3.2
ZLIB_SHA256=bb329a0a2cd0274d05519d61c667c062e06990d72e125ee2dfa8de64f0119d16
JOBS=${JOBS:-1}

apk add --no-cache build-base perl linux-headers curl ca-certificates >/dev/null

mkdir -p /build && cd /build
fetch() { # url sha256 file
  curl -fsSL --retry 3 -o "$3" "$1"
  echo "$2  $3" | sha256sum -c -
}
fetch "https://openresty.org/download/openresty-${OPENRESTY_VERSION}.tar.gz" "$OPENRESTY_SHA256" openresty.tar.gz
fetch "https://github.com/openssl/openssl/releases/download/openssl-${OPENSSL_VERSION}/openssl-${OPENSSL_VERSION}.tar.gz" "$OPENSSL_SHA256" openssl.tar.gz
fetch "https://github.com/PCRE2Project/pcre2/releases/download/pcre2-${PCRE2_VERSION}/pcre2-${PCRE2_VERSION}.tar.gz" "$PCRE2_SHA256" pcre2.tar.gz
fetch "https://github.com/madler/zlib/releases/download/v${ZLIB_VERSION}/zlib-${ZLIB_VERSION}.tar.gz" "$ZLIB_SHA256" zlib.tar.gz
for f in *.tar.gz; do tar xzf "$f"; done

cd "openresty-${OPENRESTY_VERSION}"
# OpenSSL, PCRE2 and zlib are compiled in statically from the verified sources above.
./configure -j"$JOBS" \
  --prefix=/usr/local/openresty \
  --with-cc-opt="-O2 -fstack-protector-strong -D_FORTIFY_SOURCE=2 -fPIC" \
  --with-ld-opt="-Wl,-z,relro -Wl,-z,now -pie" \
  --with-openssl="/build/openssl-${OPENSSL_VERSION}" \
  --with-openssl-opt="no-weak-ssl-ciphers no-ssl3 no-tls1 no-tls1_1 no-dtls no-quic no-comp no-legacy" \
  --with-pcre="/build/pcre2-${PCRE2_VERSION}" --with-pcre-jit \
  --with-zlib="/build/zlib-${ZLIB_VERSION}" \
  --with-compat --with-threads --with-file-aio \
  --with-http_ssl_module --with-http_v2_module --with-http_realip_module \
  --with-http_stub_status_module --with-http_gzip_static_module --with-http_gunzip_module \
  --with-http_auth_request_module --with-http_secure_link_module \
  --with-stream --with-stream_ssl_module --with-stream_ssl_preread_module --with-stream_realip_module \
  --without-mail_pop3_module --without-mail_imap_module --without-mail_smtp_module \
  --without-http_ssi_module --without-http_autoindex_module --without-http_userid_module \
  --without-http_memcached_module --without-http_empty_gif_module --without-http_browser_module \
  --without-http_uwsgi_module --without-http_scgi_module --without-http_fastcgi_module \
  --without-http_redis2_module --without-http_memc_module --without-http_rds_json_module \
  --without-http_rds_csv_module --without-lua_rds_parser --without-http_xss_module \
  --without-http_coolkit_module --without-http_form_input_module --without-http_echo_module \
  --without-lua_resty_mysql --without-lua_resty_memcached --without-lua_resty_upload \
  --without-lua_resty_websocket \
  --with-luajit-xcflags="-DLUAJIT_NUMMODE=2 -DLUAJIT_ENABLE_LUA52COMPAT"
make -j"$JOBS"
make install
rm -rf /usr/local/openresty/nginx/html /usr/local/openresty/pod
# Prove what was built.
/usr/local/openresty/bin/openresty -V
