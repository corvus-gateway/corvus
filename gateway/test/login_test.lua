-- Unit tests for corvus/login.lua (session cookie, form token, pushed state, request
-- checks). Run with plain LuaJIT from the repository root (CI lua job):
--   luajit gateway/test/login_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local crypto = require("corvus.crypto")
local ch = require("corvus.challenge")
local lg = require("corvus.login")
local eq = T.eq

local A = string.rep("a", 40)
local B = string.rep("b", 40)
local now = 1700000000

-- ---------------------------------------------------------------- settings
eq(lg.settings(nil), nil, "no login block: no login page")
local s = lg.settings({ mode = "users", hours = 24, exclude = { "/api/", "", "x", "/" }, pass_user = true })
eq(s.mode .. ":" .. s.hours .. ":" .. #s.exclude .. ":" .. s.exclude[1] .. ":" .. tostring(s.pass_user), "users:24:1:/api/:true", "users settings")
s = lg.settings({ mode = "pin", hours = 99999, pass_user = true })
eq(s.mode .. ":" .. s.hours .. ":" .. tostring(s.pass_user), "pin:720:false", "pin: hours clamped, never passes a user")
eq(lg.settings({ mode = "other" }).mode .. lg.settings({}).hours, "pin168", "unknown mode is pin, default 7 days")

-- ---------------------------------------------------------------- session cookie
eq(lg.set_secrets("short"), false, "short secret refused")
eq(lg.set_secrets(A), true, "secret accepted")
local tok = lg.make_session(3, 7, 2, 24, "photos.example.com", now)
local p = lg.parse_session(tok, 3, "photos.example.com", now + 10)
eq(p and (p.hid .. ":" .. p.uid .. ":" .. p.ver .. ":" .. p.exp), "3:7:2:" .. (now + 86400), "session round trip")
eq(tok:match("^s1%.%x+%.3%.7%.2%.%d+%.[%w_-]+$") ~= nil, true, "session format s1.<kid>.<host>.<user>.<version>.<exp>.<mac>")
eq(select(2, lg.parse_session(tok, 3, "photos.example.com", now + 86400)), "expired", "session expires")
eq(select(2, lg.parse_session(tok, 3, "other.example.com", now)), "signature", "session bound to the host name")
eq(select(2, lg.parse_session(tok, 4, "photos.example.com", now)), "host", "session bound to the host id")
eq(select(2, lg.parse_session(tok:gsub("%.7%.2%.", ".8.2."), 3, "photos.example.com", now)), "signature", "user id cannot be changed")
eq(select(2, lg.parse_session(tok:gsub("%.7%.2%.", ".7.3."), 3, "photos.example.com", now)), "signature", "version cannot be changed")
local exp = tok:match("%.(%d+)%.[%w_-]+$")
eq(select(2, lg.parse_session(tok:gsub(exp, tostring(exp + 3600)), 3, "photos.example.com", now)), "signature", "lifetime cannot be extended")
eq(select(2, lg.parse_session(tok .. "x", 3, "photos.example.com", now)), "signature", "tampered MAC")
eq(select(2, lg.parse_session("s1.zz", 3, "photos.example.com", now)), "malformed", "malformed session")
eq(select(2, lg.parse_session(nil, 3, "photos.example.com", now)), "malformed", "missing session")
eq(select(2, lg.parse_session(string.rep("x", 300), 3, "photos.example.com", now)), "malformed", "oversized session")
-- not bound to the client address: nothing about the client is in the token or the MAC
eq(lg.parse_session(tok, 3, "photos.example.com", now) ~= nil, true, "session valid from any address")
-- key rotation
lg.set_secrets(B, A)
eq(lg.parse_session(tok, 3, "photos.example.com", now) ~= nil, true, "previous key accepted during rotation")
local tok2 = lg.make_session(3, 7, 2, 24, "photos.example.com", now)
eq(tok2:match("^s1%.(%x+)") ~= tok:match("^s1%.(%x+)"), true, "new sessions signed with the new key")
lg.set_secrets(B)
eq(select(2, lg.parse_session(tok, 3, "photos.example.com", now)), "unknown key", "retired key refused")
-- separate from the challenge key derived from the same secret
ch.set_secrets(B)
local pass = ch.make_pass(0, "i", 3600, "203.0.113.5", "photos.example.com", "", now)
eq(pass:match("^p1%.(%x+)") ~= tok2:match("^s1%.(%x+)"), true, "login key separate from the challenge key")
eq(crypto.hmac_key(B).kid ~= tok2:match("^s1%.(%x+)"), true, "login key separate from the access queue key")
local pin = lg.make_session(3, 0, 5, 1, "photos.example.com", now)
eq(lg.parse_session(pin, 3, "photos.example.com", now).uid, 0, "PIN session has user 0")

-- cookies
eq(lg.session_cookie("T", 24, false), "__corvus_login=T; Path=/; Max-Age=86400; HttpOnly; SameSite=Lax", "session cookie flags (HTTP)")
eq(lg.session_cookie("T", 1, true), "__corvus_login=T; Path=/; Max-Age=3600; HttpOnly; SameSite=Lax; Secure", "Secure on HTTPS")
eq(lg.clear_cookie(true), "__corvus_login=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax; Secure", "logout clears the cookie")

-- ---------------------------------------------------------------- pushed state
eq(lg.normalize_state(nil), nil, "no state")
eq(lg.normalize_state({ users = {} }), nil, "state without hosts refused")
local st = lg.normalize_state({ users = { ["7"] = 2, ["8"] = 1, bad = 1 }, names = { ["7"] = "alice" },
  hosts = { ["3"] = 5, ["4"] = 0 }, access = { ["3"] = { 7 }, ["4"] = {} } })
eq(st.users[7] .. st.names[7] .. st.hosts[3], "2alice5", "state keyed by number")
local ok = lg.session_ok
local us = { uid = 7, ver = 2 }
eq(ok(us, 3, "users", st), true, "users: version matches and user allowed")
eq(ok({ uid = 7, ver = 1 }, 3, "users", st), false, "users: old version (signed out, password changed)")
eq(ok({ uid = 9, ver = 1 }, 3, "users", st), false, "users: missing user (deleted or disabled)")
eq(ok({ uid = 8, ver = 1 }, 3, "users", st), false, "users: user not in the host's groups")
eq(ok(us, 4, "users", st), false, "users: another host that does not allow the user")
eq(ok(us, 5, "users", st), false, "users: host without access list")
eq(ok({ uid = 0, ver = 5 }, 3, "users", st), false, "users: a PIN session does not count")
eq(ok({ uid = 0, ver = 5 }, 3, "pin", st), true, "pin: version matches")
eq(ok({ uid = 0, ver = 4 }, 3, "pin", st), false, "pin: PIN changed")
eq(ok({ uid = 7, ver = 5 }, 3, "pin", st), false, "pin: a user session does not count")
eq(ok({ uid = 0, ver = 0 }, 9, "pin", st), false, "pin: host not pushed")
eq(ok(us, 3, "users", nil), false, "nothing pushed yet: fail closed")
eq(ok(nil, 3, "users", st), false, "no session")
eq(lg.header_name("alice@example.com"), "alice@example.com", "user name as header")
eq(lg.header_name("Alice\r\nX: 1"), nil, "unsafe name refused")
eq(lg.header_name(""), nil, "empty name refused")

-- ---------------------------------------------------------------- form token
lg.set_secrets(A)
local nonce = string.rep("0f", 16)
local ft = lg.make_form(3, "photos.example.com", nonce, now)
eq(lg.check_form(ft, 3, "photos.example.com", nonce, now + 10), true, "form token accepted")
eq(select(2, lg.check_form(ft, 3, "photos.example.com", nonce, now + lg.FORM_TTL)), "expired", "form token expires after 10 minutes")
eq(select(2, lg.check_form(ft, 4, "photos.example.com", nonce, now)), "signature", "form token bound to the host id")
eq(select(2, lg.check_form(ft, 3, "other.example.com", nonce, now)), "signature", "form token bound to the host name")
eq(select(2, lg.check_form(ft, 3, "photos.example.com", string.rep("1f", 16), now)), "signature", "form token bound to the browser nonce")
eq(select(2, lg.check_form(ft, 3, "photos.example.com", nil, now)), "nonce", "no nonce cookie")
eq(select(2, lg.check_form(ft, 3, "photos.example.com", "xyz", now)), "nonce", "malformed nonce")
eq(select(2, lg.check_form("", 3, "photos.example.com", nonce, now)), "malformed", "empty token")
eq(select(2, lg.check_form(nil, 3, "photos.example.com", nonce, now)), "malformed", "missing token")
eq(select(2, lg.check_form(ft .. "x", 3, "photos.example.com", nonce, now)), "signature", "tampered token")
eq(select(2, lg.check_form(tok2, 3, "photos.example.com", nonce, now)), "malformed", "a session is not a form token")
local fexp = ft:match("^k1%.%x+%.(%d+)%.")
eq(select(2, lg.check_form(ft:gsub(fexp, tostring(fexp + 9999)), 3, "photos.example.com", nonce, now)), "signature", "form lifetime cannot be extended")

-- ---------------------------------------------------------------- request checks
eq(lg.exempt_path("/__corvus/login"), true, "own endpoints exempt")
eq(lg.exempt_path("/.well-known/acme-challenge/abc"), true, "ACME exempt")
eq(lg.exempt_path("/__corvusx"), false, "similar path not exempt")
eq(lg.exempt_path("/"), false, "root not exempt")
local ex = { "/api/", "/hooks" }
eq(lg.excluded("/api/v1/items", "/api/v1/items?x=1", ex), true, "excluded prefix")
eq(lg.excluded("/hooks/x", "/hooks/x", ex), true, "second prefix")
eq(lg.excluded("/hooksmith", "/hooksmith", ex), true, "prefix match as configured")
eq(lg.excluded("/apix", "/apix", ex), false, "not under the prefix")
eq(lg.excluded("/API/v1", "/API/v1", ex), false, "case-sensitive (stricter)")
eq(lg.excluded("/api/v1", "/api%2F..%2Fadmin", ex), false, "encoded separators refused")
eq(lg.excluded("/api/x", "/api/%2e%2e/admin", ex), false, "encoded dot segments refused")
eq(lg.excluded("/api/x", "/api\\..\\admin", ex), false, "backslashes refused")
eq(lg.excluded("/api/x", "/x/../api/x", ex), false, "raw path must start with the prefix")
eq(lg.excluded("/api/x", "/api/./x", ex), false, "dot segment in raw path refused")
eq(lg.excluded("/api/x", "/api/x?next=%2F..%2F", ex), true, "query string is not checked")
eq(lg.excluded("/api/x", "/api/x", {}), false, "no exclusions")
eq(lg.excluded("/api/x", "/api/x", nil), false, "no list")

local rt = function(r) return lg.return_target(r, ch.safe_return) end
eq(rt("/photos/2024?x=1"), "/photos/2024?x=1", "return to the original path")
eq(rt("//evil.example/"), "/", "open redirect refused")
eq(rt("https://evil.example/"), "/", "absolute URL refused")
eq(rt("/\\evil.example"), "/", "backslash refused")
eq(rt("/__corvus/logout"), "/", "never back to the gateway endpoints")
eq(rt(nil), "/", "missing return path")

eq(lg.wants_page("GET", "text/html,application/xhtml+xml"), true, "browser page request")
eq(lg.wants_page("HEAD", "TEXT/HTML"), true, "HEAD, case-insensitive")
eq(lg.wants_page("GET", "application/json"), false, "API request")
eq(lg.wants_page("GET", nil), false, "no Accept header")
eq(lg.wants_page("POST", "text/html"), false, "form post")

eq(lg.strip_cookie("a=1; __corvus_login=s1.x; b=2"), "a=1; b=2", "session cookie removed")
eq(lg.strip_cookie("__corvus_login=s1.x"), "", "only cookie removed")
eq(lg.strip_cookie({ "a=1", "__corvus_login=x" }), "a=1", "several Cookie headers")
eq(lg.strip_cookie("__corvus_login_x=1;__corvus_ch=p1"), "__corvus_login_x=1; __corvus_ch=p1", "other cookies kept")

eq(lg.safe_qr("data:image/png;base64,iVBORw0KGgo="), "data:image/png;base64,iVBORw0KGgo=", "QR data URL")
eq(lg.safe_qr("javascript:alert(1)"), "", "QR: other URLs refused")
eq(lg.safe_qr("data:image/png;base64,\"><script>"), "", "QR: markup refused")
eq(lg.safe_key("JBSWY3DPEHPK3PXP"), "JBSWY3DPEHPK3PXP", "key text")
eq(lg.safe_key("<b>"), "", "key: markup refused")
eq(lg.safe_step("abc_DEF-123"), "abc_DEF-123", "step token")
eq(lg.safe_step("a b"), "", "step: unexpected characters refused")

eq(lg.rule_tag("login_failed"), "login:failed", "event to log tag")
eq(lg.rule_tag("login_succeeded"), "login:succeeded", "success tag")
eq(lg.rule_tag("login_unavailable"), nil, "unknown event not logged")
eq(lg.rule_tag(nil), nil, "no event")

T.done("login_test")
