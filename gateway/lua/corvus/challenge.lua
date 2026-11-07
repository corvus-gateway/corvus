-- Corvus security-rule challenge (rule action "challenge"): a stateless browser check.
--
--   js     — the challenge page finds a counter so that sha256(nonce .. counter) has
--            `bits` leading zero bits (proof of work, ~2^bits hashes in the browser)
--   cookie — the page only posts its signed token back (stops clients that do not
--            run a browser: no form handling, no cookies)
--
-- Both modes POST to /__corvus/challenge/verify, which sets the pass cookie
-- `__corvus_ch` and redirects back. Requests carrying a valid pass skip challenge
-- rules (block rules still apply).
--
-- Tokens are HMAC-SHA256 signed (corvus/crypto.lua) with a key derived from the
-- access queue secret (CORVUS_WR_SECRET, plus CORVUS_WR_SECRET_PREV during rotation),
-- so the gateway keeps no per-visitor state:
--
--   challenge  c1.<kid>.<rule>.<j|c>.<bits>.<ttl>.<i|u>.<exp>.<nonce>.<mac>
--              MAC over the payload | client IP | host; valid for TOKEN_TTL seconds
--   pass       p1.<kid>.<level>.<i|u>.<exp>.<mac>
--              MAC over the payload | client IP | host [| sha256(User-Agent) for "u"];
--              level = proof-of-work bits solved (0 = cookie check)
--
-- Every value inside a token is covered by the MAC, so clients cannot lower the
-- difficulty or extend the lifetime, and a token only verifies from the client IP
-- (and host) it was issued to. Nonces are not tracked: replaying a solved challenge
-- from the same IP within its 5-minute lifetime only yields another pass for that IP.

local crypto = require "corvus.crypto"

local byte, find, sub, match = string.byte, string.find, string.sub, string.match
local tonumber, type = tonumber, type

local _M = {}

_M.COOKIE = "__corvus_ch"
_M.VERIFY_PATH = "/__corvus/challenge/verify"
_M.TOKEN_TTL = 300                 -- s; time to solve a challenge
_M.MIN_BITS, _M.MAX_BITS, _M.DEFAULT_BITS = 12, 24, 16
_M.MIN_TTL, _M.MAX_TTL, _M.DEFAULT_TTL = 300, 7 * 86400, 86400
local MAX_RETURN = 2048

local KEYS, CURRENT   -- kid → derived key; CURRENT signs new tokens

-- Same secret as the access queue, separate key: a token of one kind can never be
-- presented as the other.
local function derive(secret)
  return crypto.hmac_key(crypto.hmac(crypto.hmac_key(secret), "corvus challenge v1"))
end

-- Installs the signing keys (current + optional previous). Returns false when the
-- current secret is missing or too short.
function _M.set_secrets(cur, prev)
  if type(cur) ~= "string" or #cur < 32 then return false end
  CURRENT = derive(cur)
  KEYS = { [CURRENT.kid] = CURRENT }
  if type(prev) == "string" and #prev >= 32 then
    local k = derive(prev)
    KEYS[k.kid] = k
  end
  return true
end

-- init_by_lua (via corvus/rules.lua).
function _M.init()
  if _M.set_secrets(os.getenv("CORVUS_WR_SECRET"), os.getenv("CORVUS_WR_SECRET_PREV")) then return end
  -- corvus/waitroom.lua refuses to start without the secret; should this module ever
  -- run without it, use a random per-start key rather than a guessable one (passes
  -- then end with the next restart).
  local rrandom = require "resty.random"
  _M.set_secrets(crypto.to_hex(rrandom.bytes(32, true) or rrandom.bytes(32)))
  ngx.log(ngx.WARN, "corvus challenge: CORVUS_WR_SECRET not set, using a temporary key")
end

-- ------------------------------------------------------------------ rule settings

-- Normalises the "challenge" object of a compiled rule (rules.json) into
-- { mode = "j"|"c", name = "js"|"cookie", bits, ttl, bind = "i"|"u", level }.
function _M.settings(c)
  c = type(c) == "table" and c or {}
  local js = c.mode ~= "cookie"
  local bits = math.floor(tonumber(c.difficulty) or _M.DEFAULT_BITS)
  if bits < _M.MIN_BITS then bits = _M.MIN_BITS elseif bits > _M.MAX_BITS then bits = _M.MAX_BITS end
  local ttl = math.floor(tonumber(c.ttl) or _M.DEFAULT_TTL)
  if ttl < _M.MIN_TTL then ttl = _M.MIN_TTL elseif ttl > _M.MAX_TTL then ttl = _M.MAX_TTL end
  return {
    mode = js and "j" or "c", name = js and "js" or "cookie",
    bits = js and bits or 0, ttl = ttl,
    bind = c.bind == "ip_ua" and "u" or "i",
    level = js and bits or 0,
  }
end

-- ------------------------------------------------------------------ proof of work

-- Number of leading zero bits of a binary string.
function _M.leading_zero_bits(s)
  local n = 0
  for i = 1, #s do
    local b = byte(s, i)
    if b ~= 0 then
      while b < 128 do
        n = n + 1
        b = b * 2
      end
      return n
    end
    n = n + 8
  end
  return n
end

-- counter: decimal digits only (at most 16), as the challenge page sends it.
function _M.pow_ok(nonce, counter, bits)
  if type(nonce) ~= "string" or type(counter) ~= "string" then return false end
  if #counter < 1 or #counter > 16 or find(counter, "[^0-9]") then return false end
  return _M.leading_zero_bits(crypto.sha256_bin(nonce .. counter)) >= bits
end

-- ------------------------------------------------------------------ tokens

local function mac(key, payload, bindv)
  return crypto.b64url(crypto.hmac(key, payload .. "|" .. bindv))
end

local function ua_hash(ua)
  return crypto.to_hex(crypto.sha256_bin(ua or ""))
end

-- s: rule settings (see _M.settings); nonce: 32 hex digits.
function _M.make_challenge(rid, s, nonce, ip, host, now)
  local p = "c1." .. CURRENT.kid .. "." .. rid .. "." .. s.mode .. "." .. s.bits .. "." .. s.ttl .. "." .. s.bind
    .. "." .. (now + _M.TOKEN_TTL) .. "." .. nonce
  return p .. "." .. mac(CURRENT, p, (ip or "") .. "|" .. (host or ""))
end

-- Returns { rid, mode, bits, ttl, bind, exp, nonce } or nil, reason.
function _M.parse_challenge(tok, ip, host, now)
  if type(tok) ~= "string" or #tok > 300 then return nil, "malformed" end
  local kid, rid, mode, bits, ttl, bind, exp, nonce, sig =
    match(tok, "^c1%.(%x+)%.(%d+)%.([jc])%.(%d+)%.(%d+)%.([iu])%.(%d+)%.(%x+)%.([%w_-]+)$")
  if not kid or #nonce ~= 32 or #rid > 18 then return nil, "malformed" end
  local key = KEYS and KEYS[kid]
  if not key then return nil, "unknown key" end
  local p = sub(tok, 1, #tok - #sig - 1)
  if not crypto.const_eq(mac(key, p, (ip or "") .. "|" .. (host or "")), sig) then return nil, "signature" end
  exp = tonumber(exp)
  if now >= exp then return nil, "expired" end
  return { rid = tonumber(rid), mode = mode, bits = tonumber(bits), ttl = tonumber(ttl), bind = bind, exp = exp, nonce = nonce }
end

local function pass_bind(bind, ip, host, ua)
  local v = (ip or "") .. "|" .. (host or "")
  if bind == "u" then v = v .. "|" .. ua_hash(ua) end
  return v
end

function _M.make_pass(level, bind, ttl, ip, host, ua, now)
  local p = "p1." .. CURRENT.kid .. "." .. level .. "." .. bind .. "." .. (now + ttl)
  return p .. "." .. mac(CURRENT, p, pass_bind(bind, ip, host, ua))
end

-- Returns { level, bind, exp } or nil, reason.
function _M.parse_pass(tok, ip, host, ua, now)
  if type(tok) ~= "string" or #tok > 200 then return nil, "malformed" end
  local kid, level, bind, exp, sig = match(tok, "^p1%.(%x+)%.(%d+)%.([iu])%.(%d+)%.([%w_-]+)$")
  if not kid then return nil, "malformed" end
  local key = KEYS and KEYS[kid]
  if not key then return nil, "unknown key" end
  local p = sub(tok, 1, #tok - #sig - 1)
  if not crypto.const_eq(mac(key, p, pass_bind(bind, ip, host, ua)), sig) then return nil, "signature" end
  exp = tonumber(exp)
  if now >= exp then return nil, "expired" end
  return { level = tonumber(level), bind = bind, exp = exp }
end

-- A pass satisfies a rule when it solved at least the rule's difficulty (a cookie
-- check satisfies only cookie rules) and is bound at least as strictly.
function _M.satisfies(pass, s)
  if not pass then return false end
  if pass.level < s.level then return false end
  return s.bind == "i" or pass.bind == "u"
end

-- ------------------------------------------------------------------ redirect target

-- Only same-host relative paths: "/" followed by anything but "/" or "\" (which
-- browsers would read as a network-path reference to another host), printable
-- ASCII without spaces or backslashes, at most 2 KB. Anything else becomes "/".
function _M.safe_return(r)
  if type(r) ~= "string" or #r == 0 or #r > MAX_RETURN then return "/" end
  if sub(r, 1, 1) ~= "/" then return "/" end
  local c2 = sub(r, 2, 2)
  if c2 == "/" or c2 == "\\" then return "/" end
  if find(r, "[^\33-\126]") or find(r, "\\", 1, true) then return "/" end
  return r
end

-- ------------------------------------------------------------------ request helpers

local function client()
  return ngx.var.remote_addr or "", ngx.var.host or "", ngx.var.http_user_agent or ""
end

-- The request's valid pass (see parse_pass) or nil. The cookie is verified once per
-- request (cache: the rule engine's per-request ctx table).
function _M.pass(ctx)
  local p = ctx.__corvus_ch
  if p == nil then
    local tok = ngx.var["cookie_" .. _M.COOKIE]
    p = false
    if tok then
      local ip, host, ua = client()
      p = _M.parse_pass(tok, ip, host, ua, ngx.time()) or false
    end
    ctx.__corvus_ch = p
  end
  return p or nil
end

-- Whether the request carries a pass satisfying rule settings s.
function _M.passed(ctx, s)
  return _M.satisfies(_M.pass(ctx), s)
end

-- Placeholder values for the challenge page (see corvus/rules.lua send_page).
function _M.page_vars(rid, s, ret)
  local rrandom = require "resty.random"
  local nonce = crypto.to_hex(rrandom.bytes(16, true) or rrandom.bytes(16))
  local ip, host = client()
  return {
    TOKEN = _M.make_challenge(rid, s, nonce, ip, host, ngx.time()),
    MODE = s.name, BITS = s.bits, RETURN = _M.safe_return(ret),
  }
end

-- Where to send the client after the challenge: the original URL for GET/HEAD,
-- otherwise the site root (a POST body cannot be replayed).
function _M.return_path()
  local m = ngx.req.get_method()
  if m == "GET" or m == "HEAD" then return _M.safe_return(ngx.var.request_uri) end
  return "/"
end

-- Verifies a POSTed challenge. Returns
--   true,  rid, cookie            — solved: set `cookie` and redirect
--   false, rid|nil, reason        — failed (rid when the token names a rule)
-- `rule_ok(rid)` reports whether rid is a challenge rule of the current host.
function _M.verify(args, rule_ok)
  local t, c = args.t, args.c
  if type(t) == "table" then t = t[1] end
  if type(c) == "table" then c = c[1] end
  local ip, host, ua = client()
  local now = ngx.time()
  local tok, why = _M.parse_challenge(t, ip, host, now)
  if not tok then
    local rid = type(t) == "string" and tonumber(match(t, "^c1%.%x+%.(%d+)%.") or "") or nil
    if rid and not rule_ok(rid) then rid = nil end
    return false, rid, why
  end
  if not rule_ok(tok.rid) then return false, nil, "rule" end
  if tok.mode == "j" and not _M.pow_ok(tok.nonce, c, tok.bits) then return false, tok.rid, "work" end
  local level = tok.mode == "j" and tok.bits or 0
  local cookie = _M.COOKIE .. "=" .. _M.make_pass(level, tok.bind, tok.ttl, ip, host, ua, now)
    .. "; Path=/; Max-Age=" .. tok.ttl .. "; HttpOnly; SameSite=Lax"
  if ngx.var.scheme == "https" then cookie = cookie .. "; Secure" end
  return true, tok.rid, cookie
end

return _M
