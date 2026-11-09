-- Access-queue tokens (corvus/waitroom.lua): binding to the browser family and,
-- optionally, the client network, so a copied cookie does not let someone else skip
-- the line. Run with plain LuaJIT from the repository root (CI lua job):
--   luajit gateway/test/waitroom_token_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
package.loaded["resty.redis"] = {}
package.loaded["cjson.safe"] = {}
package.loaded["resty.string"] = {}
local eq = T.eq
local wr = require("corvus.waitroom")
local crypto = require("corvus.crypto")

local fam = wr._ua_family
local chrome130 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
local chrome131 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
local firefox = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0"
local iphone = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
eq(fam(chrome130), fam(chrome131), "a browser update keeps the family")
eq(fam(chrome130) ~= fam(firefox), true, "another browser is another family")
eq(fam(chrome130) ~= fam(iphone), true, "another device is another family")
eq(fam(nil), "", "no User-Agent")

wr._set_secret(string.rep("k", 40))
local now = 1700000000
T.set_time(now)
local sid = string.rep("ab", 16)
local room = { name = "shop", bind_ip = false }
local netroom = { name = "shop", bind_ip = true }

local function as(ua, ip)
  ngx.ctx = {}
  ngx.var = { http_user_agent = ua, remote_addr = ip }
end

-- same browser family, other network: accepted (no network binding)
as(chrome130, "203.0.113.5")
local tok = wr._make_token("q", "shop", sid, now, now + 3600, wr._binding(room))
eq(tok:match("^v3%.") ~= nil, true, "new tokens are v3")
as(chrome131, "198.51.100.7")
local p = wr._parse_token(tok, "q", "shop", wr._binding(room))
eq(p and p.sid, sid, "same browser family after an update and a network change")
eq(p and p.legacy, false, "v3 is not legacy")

-- copied into another browser or device: rejected
as(firefox, "203.0.113.5")
eq(wr._parse_token(tok, "q", "shop", wr._binding(room)), nil, "copied to another browser")
as(iphone, "203.0.113.5")
eq(wr._parse_token(tok, "q", "shop", wr._binding(room)), nil, "copied to another device")

-- network binding (room option): same /24 accepted, another network rejected
as(chrome130, "203.0.113.5")
local ntok = wr._make_token("a", "shop", sid, now, now + 120, wr._binding(netroom))
as(chrome130, "203.0.113.200")
eq(wr._parse_token(ntok, "a", "shop", wr._binding(netroom)) ~= nil, true, "same /24")
as(chrome130, "198.51.100.7")
eq(wr._parse_token(ntok, "a", "shop", wr._binding(netroom)), nil, "another network")
as(chrome130, "2001:db8:1:2::1")
local v6 = wr._make_token("q", "shop", sid, now, now + 3600, wr._binding(netroom))
as(chrome130, "2001:db8:1:ffff::9")
eq(wr._parse_token(v6, "q", "shop", wr._binding(netroom)) ~= nil, true, "same IPv6 /48")

-- tampering, kind, room and expiry
as(chrome130, "203.0.113.5")
eq(wr._parse_token(tok:gsub("%.q%.", ".a."), "a", "shop", wr._binding(room)), nil, "kind changed")
eq(wr._parse_token(tok, "a", "shop", wr._binding(room)), nil, "queue token is not an admission token")
eq(wr._parse_token(tok, "q", "other", wr._binding(room)), nil, "other room")
T.set_time(now + 3600)
eq(wr._parse_token(tok, "q", "shop", wr._binding(room)), nil, "expired")
T.set_time(now)

-- v2 tokens from before the upgrade: accepted until they expire, marked legacy
local key = crypto.hmac_key(string.rep("k", 40))
local function v2(kind, bind)
  local pl = "v2." .. key.kid .. "." .. kind .. ".shop." .. sid .. "." .. now .. "." .. (now + 600)
  return pl .. "." .. crypto.b64url(crypto.hmac(key, pl .. (bind and ("|" .. bind) or "")))
end
p = wr._parse_token(v2("q"), "q", "shop", wr._binding(room), nil)
eq(p and p.legacy, true, "v2 queue token still accepted, marked legacy")
p = wr._parse_token(v2("a", "203.0.113"), "a", "shop", wr._binding(netroom), "203.0.113")
eq(p and p.sid, sid, "v2 network-bound admission token from the same network")
eq(wr._parse_token(v2("a", "203.0.113"), "a", "shop", wr._binding(netroom), "198.51.100"), nil, "v2 admission token from another network")

T.done("waitroom_token_test")
