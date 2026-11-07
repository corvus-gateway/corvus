-- Unit tests for corvus/underattack.lua (is the mode active, is the request exempt,
-- does its pass satisfy the host's settings). Run with plain LuaJIT from the
-- repository root (CI lua job):
--   luajit gateway/test/underattack_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local ch = require("corvus.challenge")
local ua = require("corvus.underattack")
local eq = T.eq

ch.set_secrets(string.rep("k", 32))
local now = 1700000000

-- ---------------------------------------------------------------- settings
eq(ua.settings(nil), nil, "settings: mode off (no object)")
local on = ua.settings({ on = true, mode = "cookie", difficulty = 16, ttl = 600, bind = "ip" })
eq(on.on, true, "settings: on")
eq(on.ch.name .. ":" .. on.ch.level .. ":" .. on.ch.ttl .. ":" .. on.ch.bind, "cookie:0:600:i", "settings: cookie challenge")
local auto = ua.settings({ on = false, mode = "js", difficulty = 16, ttl = 3600, bind = "ip_ua" })
eq(auto.on, false, "settings: auto")
eq(auto.ch.name .. ":" .. auto.ch.level .. ":" .. auto.ch.ttl .. ":" .. auto.ch.bind, "js:16:3600:u", "settings: js challenge")
eq(ua.settings({}).ch.name .. ":" .. ua.settings({}).ch.bits, "js:16", "settings: defaults")

-- ---------------------------------------------------------------- active
eq(ua.active(nil, now + 60, now), false, "active: mode off, even with a stray activation")
eq(ua.active(on, nil, now), true, "active: on needs no activation")
eq(ua.active(auto, nil, now), false, "active: auto without activation")
eq(ua.active(auto, now + 60, now), true, "active: auto with a running activation")
eq(ua.active(auto, now, now), false, "active: activation ended")
eq(ua.active(auto, "garbage", now), false, "active: unreadable activation")

-- ---------------------------------------------------------------- exempt paths
eq(ua.exempt_path("/.well-known/acme-challenge/abc"), true, "exempt: ACME")
eq(ua.exempt_path("/__corvus/challenge/verify"), true, "exempt: challenge verify")
eq(ua.exempt_path("/__corvus/wr/status"), true, "exempt: access queue API")
eq(ua.exempt_path("/__corvus/wait/shop"), true, "exempt: access queue page")
eq(ua.exempt_path("/"), false, "not exempt: root")
eq(ua.exempt_path("/.well-known/security.txt"), false, "not exempt: other .well-known")
eq(ua.exempt_path("/__corvusx"), false, "not exempt: lookalike")
eq(ua.exempt_path(nil), false, "not exempt: no path")

-- ---------------------------------------------------------------- decide
local ip, host, agent = "203.0.113.5", "example.com", "Mozilla/5.0"
local js16 = ch.parse_pass(ch.make_pass(16, "i", 600, ip, host, agent, now), ip, host, agent, now)
local js16u = ch.parse_pass(ch.make_pass(16, "u", 600, ip, host, agent, now), ip, host, agent, now)
local cookie = ch.parse_pass(ch.make_pass(0, "i", 600, ip, host, agent, now), ip, host, agent, now)
eq(js16 ~= nil and cookie ~= nil and js16u ~= nil, true, "test passes parse")

eq(ua.decide(nil, nil, now, "/", false, nil), "off", "decide: mode off")
eq(ua.decide(auto, nil, now, "/", false, nil), "off", "decide: auto, not activated")
eq(ua.decide(auto, now - 1, now, "/", false, nil), "off", "decide: auto, activation over")
eq(ua.decide(on, nil, now, "/shop", false, nil), "challenge", "decide: on, no pass → challenge")
eq(ua.decide(auto, now + 900, now, "/shop", false, nil), "challenge", "decide: auto active, no pass → challenge")
eq(ua.decide(on, nil, now, "/.well-known/acme-challenge/t", false, nil), "exempt", "decide: ACME exempt")
eq(ua.decide(on, nil, now, "/__corvus/challenge/verify", false, nil), "exempt", "decide: verify endpoint exempt")
eq(ua.decide(on, nil, now, "/shop", true, nil), "exempt", "decide: exempt client address")
eq(ua.decide(on, nil, now, "/shop", false, cookie), "pass", "decide: cookie pass satisfies cookie mode")
eq(ua.decide(on, nil, now, "/shop", false, js16), "pass", "decide: js pass satisfies cookie mode")
eq(ua.decide(auto, now + 900, now, "/shop", false, cookie), "challenge", "decide: cookie pass does not satisfy js mode")
eq(ua.decide(auto, now + 900, now, "/shop", false, js16), "challenge", "decide: IP-bound pass does not satisfy IP+UA binding")
eq(ua.decide(auto, now + 900, now, "/shop", false, js16u), "pass", "decide: js IP+UA pass satisfies")

-- an expired or foreign pass parses to nil and is challenged
eq(ch.parse_pass(ch.make_pass(16, "u", 600, ip, host, agent, now), ip, host, agent, now + 600), nil, "pass expired")
eq(ua.decide(auto, now + 900, now, "/shop", false, ch.parse_pass(ch.make_pass(16, "u", 600, ip, "other.example", agent, now), ip, host, agent, now)),
  "challenge", "decide: pass of another host")

-- the virtual rule id round-trips through a challenge token
local tok = ch.make_challenge(ua.RULE, on.ch, "0123456789abcdef0123456789abcdef", ip, host, now)
eq(ch.parse_challenge(tok, ip, host, now).rid, 0, "challenge token carries rule id 0")

T.done("underattack_test")
