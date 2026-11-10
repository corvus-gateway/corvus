-- Access-queue HTTP helpers (corvus/waitroom.lua): the same-origin check of /leave and
-- the response headers when Corvus adds a cookie. Run with plain LuaJIT from the
-- repository root (CI lua job):
--   luajit gateway/test/waitroom_http_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
package.loaded["resty.redis"] = {}
package.loaded["cjson.safe"] = {}
package.loaded["resty.string"] = {}
local eq = T.eq
local wr = require("corvus.waitroom")

-- ---------------------------------------------------------------- /leave origin
local function so(origin, sfs, host)
  ngx.var = { http_origin = origin, http_sec_fetch_site = sfs, http_host = host or "shop.example" }
  return wr._same_origin()
end
eq(so("https://shop.example"), true, "same host")
eq(so("https://SHOP.example"), true, "host case")
eq(so("https://shop.example:8443", nil, "shop.example:8443"), true, "same host and port")
eq(so("https://evil.example"), false, "other host")
eq(so("https://shop.example.evil.example"), false, "host suffix")
eq(so("https://shop.example/x"), false, "origin with a path")
eq(so(nil), false, "no Origin")
eq(so(""), false, "empty Origin")
eq(so("null"), false, "opaque Origin")
eq(so(nil, "same-origin"), true, "no Origin, Sec-Fetch-Site same-origin")
eq(so("null", "same-origin"), true, "opaque Origin, Sec-Fetch-Site same-origin")
eq(so(nil, "cross-site"), false, "Sec-Fetch-Site cross-site")
eq(so(nil, "same-site"), false, "Sec-Fetch-Site same-site")
eq(so("https://evil.example", "same-origin"), false, "a foreign Origin is refused")

-- ---------------------------------------------------------------- cookie responses
local function filter(cookies, hdr)
  ngx.ctx = { corvus_cookies = cookies }
  ngx.header = hdr
  wr.header_filter()
  return ngx.header
end
local h = filter(nil, { ["Cache-Control"] = "public, max-age=600" })
eq(h["Cache-Control"], "public, max-age=600", "no Corvus cookie: upstream caching kept")
eq(h["Set-Cookie"], nil, "no Corvus cookie: no Set-Cookie")
h = filter({ "corvus_q=1" }, { ["Cache-Control"] = "public, max-age=600", ["Set-Cookie"] = "app=1" })
eq(h["Cache-Control"], "private, no-store", "Corvus cookie: not stored by shared caches")
eq(type(h["Set-Cookie"]), "table", "cookies merged")
eq(h["Set-Cookie"][1], "app=1", "upstream cookie first")
eq(h["Set-Cookie"][2], "corvus_q=1", "Corvus cookie added")
h = filter({ "corvus_a=1" }, {})
eq(h["Cache-Control"], "private, no-store", "Corvus cookie without upstream Cache-Control")

T.done("waitroom_http_test")
