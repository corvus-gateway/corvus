-- Unit tests for corvus/crypto.lua and corvus/challenge.lua (tokens, proof of work,
-- redirect targets). Run with plain LuaJIT from the repository root (CI lua job):
--   luajit gateway/test/challenge_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local crypto = require("corvus.crypto")
local ch = require("corvus.challenge")
local eq = T.eq
local hex = crypto.to_hex

-- ---------------------------------------------------------------- primitives
eq(hex("\0\1\15\16\127\128\255"), "00010f107f80ff", "to_hex byte values")
eq(hex(""), "", "to_hex empty")
eq(hex(crypto.sha256_bin("")), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "sha256 empty")
eq(hex(crypto.sha256_bin("abc")), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", "sha256 abc")
eq(hex(crypto.sha256_bin(string.rep("a", 1000))), "41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3", "sha256 multi-block")
-- RFC 4231 test case 2
eq(hex(crypto.hmac(crypto.hmac_key("Jefe"), "what do ya want for nothing?")),
  "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843", "HMAC-SHA256 RFC 4231 #2")
eq(crypto.const_eq("abc", "abc"), true, "const_eq equal")
eq(crypto.const_eq("abc", "abd"), false, "const_eq differs")
eq(crypto.const_eq("abc", "ab"), false, "const_eq length")
eq(crypto.const_eq("abc", nil), false, "const_eq nil")
eq(crypto.b64url("\251\255\254"), "-__-", "b64url alphabet, no padding")

-- ---------------------------------------------------------------- leading zero bits
local lzb = ch.leading_zero_bits
eq(lzb(""), 0, "lzb empty")
eq(lzb("\128"), 0, "lzb 0x80")
eq(lzb("\1"), 7, "lzb 0x01")
eq(lzb("\0\15"), 12, "lzb 0x000f")
eq(lzb("\0\0\255"), 16, "lzb two zero bytes")
eq(lzb("\0\0\0\1"), 31, "lzb 31")
eq(lzb(string.rep("\0", 32)), 256, "lzb all zero")

-- ---------------------------------------------------------------- proof of work
local nonce = "0123456789abcdef0123456789abcdef"
local function zbits(n) return lzb(crypto.sha256_bin(nonce .. n)) end
-- first counter with at least 8 zero bits, and one with exactly k bits
local found
for n = 0, 100000 do if zbits(n) >= 8 then found = n break end end
eq(found ~= nil, true, "a counter with 8 zero bits exists")
local k = zbits(found)
eq(ch.pow_ok(nonce, tostring(found), 8), true, "pow: solution accepted")
eq(ch.pow_ok(nonce, tostring(found), k), true, "pow: exactly k bits accepted at k")
eq(ch.pow_ok(nonce, tostring(found), k + 1), false, "pow: rejected at k+1 bits")
local miss
for n = 0, 1000 do if zbits(n) == 0 then miss = n break end end
eq(ch.pow_ok(nonce, tostring(miss), 1), false, "pow: no zero bit rejected at 1")
eq(ch.pow_ok(nonce, tostring(miss), 0), true, "pow: difficulty 0 accepts any counter")
eq(ch.pow_ok(nonce, "", 0), false, "pow: empty counter")
eq(ch.pow_ok(nonce, "12a", 0), false, "pow: non-digit counter")
eq(ch.pow_ok(nonce, "-1", 0), false, "pow: negative counter")
eq(ch.pow_ok(nonce, "1 ", 0), false, "pow: trailing space")
eq(ch.pow_ok(nonce, string.rep("1", 17), 0), false, "pow: counter too long")
eq(ch.pow_ok(nonce, string.rep("1", 16), 0), true, "pow: 16 digits allowed")
eq(ch.pow_ok(nonce, nil, 0), false, "pow: missing counter")
eq(ch.pow_ok(nonce, { "1" }, 0), false, "pow: table counter")

-- ---------------------------------------------------------------- settings
local s = ch.settings({ mode = "js", difficulty = 16, ttl = 3600, bind = "ip" })
eq(s.mode .. s.bits .. s.ttl .. s.bind .. s.level .. s.name, "j163600i16js", "js settings")
s = ch.settings({ mode = "cookie", difficulty = 20, ttl = 60, bind = "ip_ua" })
eq(s.mode .. s.bits .. s.ttl .. s.bind .. s.level .. s.name, "c0300u0cookie", "cookie settings, ttl clamped up")
s = ch.settings({ difficulty = 99, ttl = 10 ^ 9 })
eq(s.mode .. s.bits .. s.ttl, "j24604800", "difficulty/ttl clamped down, js by default")
s = ch.settings(nil)
eq(s.bits .. ":" .. s.ttl .. ":" .. s.bind, "16:86400:i", "defaults")
eq(ch.settings({ difficulty = 3 }).bits, 12, "difficulty clamped up")

-- ---------------------------------------------------------------- challenge tokens
eq(ch.set_secrets("short"), false, "short secret refused")
eq(ch.set_secrets(nil), false, "missing secret refused")
local A = string.rep("a", 40)
local B = string.rep("b", 40)
eq(ch.set_secrets(A), true, "secret accepted")
local now = 1700000000
local js16 = ch.settings({ difficulty = 16, ttl = 3600 })
local tok = ch.make_challenge(7, js16, nonce, "203.0.113.5", "example.com", now)
local c = ch.parse_challenge(tok, "203.0.113.5", "example.com", now + 10)
eq(c and c.rid, 7, "challenge round trip: rule")
eq(c and (c.mode .. c.bits .. ":" .. c.ttl .. c.bind .. c.nonce), "j16:3600i" .. nonce, "challenge round trip: settings")
eq(c and c.exp, now + ch.TOKEN_TTL, "challenge expires after TOKEN_TTL")
eq(select(2, ch.parse_challenge(tok, "203.0.113.6", "example.com", now)), "signature", "challenge bound to IP")
eq(select(2, ch.parse_challenge(tok, "203.0.113.5", "other.example", now)), "signature", "challenge bound to host")
eq(select(2, ch.parse_challenge(tok, "203.0.113.5", "example.com", now + ch.TOKEN_TTL)), "expired", "challenge expires")
eq(select(2, ch.parse_challenge(tok:gsub("%.j%.16%.", ".j.12."), "203.0.113.5", "example.com", now)), "signature", "lowered difficulty detected")
eq(select(2, ch.parse_challenge(tok:gsub("%.3600%.", ".604800."), "203.0.113.5", "example.com", now)), "signature", "longer pass lifetime detected")
eq(select(2, ch.parse_challenge(tok .. "x", "203.0.113.5", "example.com", now)), "signature", "tampered MAC")
eq(select(2, ch.parse_challenge("c1.zz", "203.0.113.5", "example.com", now)), "malformed", "malformed token")
eq(select(2, ch.parse_challenge(nil, "203.0.113.5", "example.com", now)), "malformed", "missing token")
eq(select(2, ch.parse_challenge(string.rep("x", 400), "203.0.113.5", "example.com", now)), "malformed", "oversized token")
-- key rotation: tokens signed with the previous secret still verify, then expire with it
ch.set_secrets(B, A)
eq(ch.parse_challenge(tok, "203.0.113.5", "example.com", now) ~= nil, true, "previous key accepted during rotation")
ch.set_secrets(B)
eq(select(2, ch.parse_challenge(tok, "203.0.113.5", "example.com", now)), "unknown key", "retired key refused")
-- the derived key differs from the access queue's HMAC key for the same secret
eq(crypto.hmac_key(B).kid ~= tok:match("^c1%.(%x+)"), true, "challenge key separate from access queue key")

-- ---------------------------------------------------------------- pass cookies
local ua = "Mozilla/5.0 test"
local p = ch.make_pass(16, "i", 3600, "203.0.113.5", "example.com", ua, now)
local pp = ch.parse_pass(p, "203.0.113.5", "example.com", "curl/8", now + 1)
eq(pp and (pp.level .. pp.bind .. ":" .. pp.exp), "16i:" .. (now + 3600), "pass round trip (IP binding ignores UA)")
eq(select(2, ch.parse_pass(p, "198.51.100.1", "example.com", ua, now)), "signature", "pass not valid from another IP")
eq(select(2, ch.parse_pass(p, "203.0.113.5", "other.example", ua, now)), "signature", "pass not valid on another host")
eq(select(2, ch.parse_pass(p, "203.0.113.5", "example.com", ua, now + 3600)), "expired", "pass expires")
eq(select(2, ch.parse_pass(p:gsub("^p1%.(%x+)%.16%.", "p1.%1.24."), "203.0.113.5", "example.com", ua, now)), "signature", "raised level detected")
local pu = ch.make_pass(0, "u", 3600, "203.0.113.5", "example.com", ua, now)
eq(ch.parse_pass(pu, "203.0.113.5", "example.com", ua, now) ~= nil, true, "IP+UA pass with the same UA")
eq(select(2, ch.parse_pass(pu, "203.0.113.5", "example.com", "Other/1", now)), "signature", "IP+UA pass refused for another UA")
eq(select(2, ch.parse_pass(pu:gsub("%.u%.", ".i."), "203.0.113.5", "example.com", ua, now)), "signature", "binding downgrade detected")
eq(select(2, ch.parse_pass("p1.x.1.i.1.", "203.0.113.5", "example.com", ua, now)), "malformed", "malformed pass")

local sat = ch.satisfies
local cookieIP, js16IP = ch.settings({ mode = "cookie" }), ch.settings({ difficulty = 16 })
local js20UA = ch.settings({ difficulty = 20, bind = "ip_ua" })
eq(sat(nil, cookieIP), false, "no pass")
eq(sat({ level = 0, bind = "i" }, cookieIP), true, "cookie pass satisfies cookie rule")
eq(sat({ level = 0, bind = "i" }, js16IP), false, "cookie pass does not satisfy js rule")
eq(sat({ level = 16, bind = "i" }, js16IP), true, "js pass satisfies equal difficulty")
eq(sat({ level = 16, bind = "i" }, cookieIP), true, "js pass satisfies cookie rule")
eq(sat({ level = 24, bind = "i" }, js20UA), false, "IP-bound pass does not satisfy IP+UA rule")
eq(sat({ level = 20, bind = "u" }, js20UA), true, "IP+UA pass satisfies IP+UA rule")
eq(sat({ level = 16, bind = "u" }, js20UA), false, "lower difficulty refused")

-- ---------------------------------------------------------------- redirect targets
local sr = ch.safe_return
eq(sr("/"), "/", "root")
eq(sr("/shop/cart?x=1&y=%2F"), "/shop/cart?x=1&y=%2F", "path with query")
eq(sr("//evil.example/"), "/", "network-path reference")
eq(sr("/\\evil.example/"), "/", "backslash network path")
eq(sr("/a\\b"), "/", "backslash anywhere")
eq(sr("https://evil.example/"), "/", "absolute URL")
eq(sr("javascript:alert(1)"), "/", "javascript URL")
eq(sr("evil.example"), "/", "relative without slash")
eq(sr("/a b"), "/", "space")
eq(sr("/a\r\nSet-Cookie: x=1"), "/", "header injection")
eq(sr("/\t/evil.example"), "/", "tab after slash")
eq(sr("/caf\195\169"), "/", "raw non-ASCII")
eq(sr("/" .. string.rep("a", 3000)), "/", "too long")
eq(sr(""), "/", "empty")
eq(sr(nil), "/", "missing")
eq(sr({ "/x" }), "/", "table")

-- ---------------------------------------------------------------- verify
ch.set_secrets(A)
T.set_time(now)
ngx.var = { remote_addr = "203.0.113.5", host = "example.com", http_user_agent = ua, scheme = "http" }
local rules = { [7] = true }
local function rule_ok(id) return rules[id] == true end
local s8 = ch.settings({ difficulty = 12, ttl = 600 })
local t8 = ch.make_challenge(7, s8, nonce, "203.0.113.5", "example.com", now)
local sol
for n = 0, 200000 do if zbits(n) >= 12 then sol = tostring(n) break end end
local ok, rid, res = ch.verify({ t = t8, c = sol }, rule_ok)
eq(ok, true, "verify: solved")
eq(rid, 7, "verify: rule id")
eq(type(res) == "string" and res:match("^__corvus_ch=p1%.%x+%.12%.i%.%d+%.[%w_-]+; Path=/; Max%-Age=600; HttpOnly; SameSite=Lax$") ~= nil, true, "verify: cookie flags (HTTP)")
local cookie = res and res:match("^__corvus_ch=([^;]+)")
eq(ch.parse_pass(cookie, "203.0.113.5", "example.com", ua, now) ~= nil, true, "verify: issued pass parses")
ngx.var.scheme = "https"
ok, rid, res = ch.verify({ t = { t8, "x" }, c = { sol } }, rule_ok)
eq(ok and res:match("; Secure$") ~= nil, true, "verify: Secure on HTTPS, first of repeated args")
ok, rid, res = ch.verify({ t = t8, c = "0" .. sol .. "1" }, rule_ok)
eq(tostring(ok) .. ":" .. tostring(rid) .. ":" .. tostring(res), "false:7:work", "verify: wrong counter")
ok, rid, res = ch.verify({ t = t8 }, rule_ok)
eq(tostring(ok) .. ":" .. tostring(res), "false:work", "verify: missing counter")
rules[7] = nil
ok, rid, res = ch.verify({ t = t8, c = sol }, rule_ok)
eq(tostring(ok) .. ":" .. tostring(rid) .. ":" .. tostring(res), "false:nil:rule", "verify: rule no longer a challenge rule")
rules[7] = true
ngx.var.remote_addr = "198.51.100.9"
ok, rid, res = ch.verify({ t = t8, c = sol }, rule_ok)
eq(tostring(ok) .. ":" .. tostring(rid) .. ":" .. tostring(res), "false:7:signature", "verify: token from another IP (rule kept for re-challenge)")
ngx.var.remote_addr = "203.0.113.5"
local tc = ch.make_challenge(7, ch.settings({ mode = "cookie" }), nonce, "203.0.113.5", "example.com", now)
ok, rid, res = ch.verify({ t = tc }, rule_ok)
eq(ok and res:match("^__corvus_ch=p1%.%x+%.0%.i%.") ~= nil, true, "verify: cookie mode needs no work")
T.set_time(now + ch.TOKEN_TTL)
ok, rid, res = ch.verify({ t = tc }, rule_ok)
eq(tostring(ok) .. ":" .. tostring(res), "false:expired", "verify: expired challenge")
ok, rid, res = ch.verify({}, rule_ok)
eq(tostring(ok) .. ":" .. tostring(rid) .. ":" .. tostring(res), "false:nil:malformed", "verify: no token")

T.done("challenge_test")
