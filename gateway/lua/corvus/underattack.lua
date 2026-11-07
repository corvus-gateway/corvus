-- Corvus "under attack" mode: every visitor of a proxy host must pass a challenge
-- (corvus/challenge.lua) before reaching the site.
--
--   on    — always active (host setting in rules.json)
--   auto  — active while corvus-control has pushed an activation for the host into the
--           shared dict corvus_ua ("h:<host id>" → end of the activation, unix time;
--           the entry also expires on its own)
--
-- The challenge is the security-rule challenge with the host's settings and the virtual
-- rule id 0: same pages, same verify endpoint, same pass cookie. Log field
-- $corvus_rule: "ua:challenge" / "ua:challenge_pass" / "ua:challenge_fail".
--
-- Order in the rewrite phase (corvus/rules.lua): bans, security rules (block, allow,
-- challenge), attack-pattern and body blocking, then this mode, then — in later
-- phases — rate limiting, Basic auth and the access queue. Requests an allow rule
-- matched never reach it. Exempt: certificate validation (/.well-known/acme-challenge/),
-- the gateway's own /__corvus/ endpoints (challenge verify, access queue) and client
-- addresses in the automatic-ban exemptions (pushed with the activations).
--
-- This module only holds the pure decision; the request plumbing is in corvus/rules.lua.

local challenge = require "corvus.challenge"

local sub, type, tonumber = string.sub, type, tonumber

local _M = {}

_M.RULE = 0     -- rule id inside challenge tokens
_M.TAG = "ua"   -- rule part of $corvus_rule

-- Host settings from rules.json ("under_attack": {on, mode, difficulty, ttl, bind}) →
-- { on = bool, ch = challenge settings } or nil when the mode is off.
function _M.settings(c)
  if type(c) ~= "table" then return nil end
  return { on = c.on == true, ch = challenge.settings(c) }
end

-- Whether the mode is active: always for "on", for "auto" while the pushed activation
-- (until, unix time) has not ended.
function _M.active(s, until_, now)
  if not s then return false end
  if s.on then return true end
  until_ = tonumber(until_)
  return until_ ~= nil and until_ > now
end

-- Paths never challenged by the mode.
function _M.exempt_path(uri)
  if type(uri) ~= "string" then return false end
  return sub(uri, 1, 28) == "/.well-known/acme-challenge/" or sub(uri, 1, 10) == "/__corvus/"
end

-- Decision for one request:
--   "off"       — mode not active for the host
--   "exempt"    — exempt path or client address
--   "pass"      — the request carries a pass satisfying the host's settings
--   "challenge" — serve the challenge page
-- s: _M.settings(); until_: pushed activation end or nil; ip_exempt: client address in
-- the exempt networks; pass: the request's parsed pass cookie (challenge.parse_pass) or nil.
function _M.decide(s, until_, now, uri, ip_exempt, pass)
  if not _M.active(s, until_, now) then return "off" end
  if ip_exempt or _M.exempt_path(uri) then return "exempt" end
  if challenge.satisfies(pass, s.ch) then return "pass" end
  return "challenge"
end

return _M
