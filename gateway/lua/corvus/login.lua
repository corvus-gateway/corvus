-- Corvus login page in front of a proxy host (#294): pure helpers for the session cookie,
-- the form token and the request checks. The request plumbing (redirects, the page,
-- the call to corvus-control) is in corvus/rules.lua.
--
-- The gateway never sees password hashes or TOTP secrets: the form goes to corvus-control
-- over its sign-in socket, and on "ok" the gateway signs its own session cookie:
--
--   session  s1.<kid>.<host id>.<user id>.<version>.<exp>.<mac>
--            MAC over the payload | host name; user id 0 for a shared PIN. Not bound to
--            the client address (a phone that moves between networks stays signed in).
--   form     k1.<kid>.<exp>.<mac>
--            MAC over the payload | "form" | host id | host name | nonce, where the nonce
--            is a random value in a short-lived cookie (__corvus_lt, set with the page),
--            so a token only works in the browser that loaded the page (no login CSRF).
--
-- Keys are HMAC-SHA256 keys derived from the access queue secret (CORVUS_WR_SECRET, plus
-- CORVUS_WR_SECRET_PREV during rotation) with their own label, separate from the
-- challenge and access queue keys.
--
-- A session is valid only while its version equals the one corvus-control pushed for the
-- user (users mode, and the user must still be allowed on the host) or for the host's PIN
-- (PIN mode). Disabling or deleting a user, changing a password or PIN, resetting two-step
-- sign-in and "end sessions" change or remove that version (see normalize_state).

local crypto = require "corvus.crypto"

local find, sub, match, lower = string.find, string.sub, string.match, string.lower
local tonumber, type, pairs, ipairs = tonumber, type, pairs, ipairs

local _M = {}

_M.COOKIE = "__corvus_login"
_M.NONCE_COOKIE = "__corvus_lt"
_M.PAGE = "/__corvus/login"
_M.LOGOUT = "/__corvus/logout"
_M.FORM_TTL = 600            -- s; the sign-in form must be sent within 10 minutes
_M.MAX_HOURS = 720           -- hours; longest session corvus-control allows (30 days)

local KEYS, CURRENT   -- kid → derived key; CURRENT signs new tokens

local function derive(secret)
  return crypto.hmac_key(crypto.hmac(crypto.hmac_key(secret), "corvus login v1"))
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
  -- without the secret, a random per-start key: sessions then end with the next restart
  local rrandom = require "resty.random"
  _M.set_secrets(crypto.to_hex(rrandom.bytes(32, true) or rrandom.bytes(32)))
  ngx.log(ngx.WARN, "corvus login: CORVUS_WR_SECRET not set, using a temporary key")
end

local function mac(key, payload, bindv)
  return crypto.b64url(crypto.hmac(key, payload .. "|" .. bindv))
end

-- ------------------------------------------------------------------ host settings

-- Normalises the "login" object of a host in rules.json:
-- { mode = "pin"|"users", hours, exclude = { prefixes }, pass_user } or nil.
function _M.settings(c)
  if type(c) ~= "table" then return nil end
  local mode = c.mode == "users" and "users" or "pin"
  local hours = math.floor(tonumber(c.hours) or 168)
  if hours < 1 then hours = 1 elseif hours > _M.MAX_HOURS then hours = _M.MAX_HOURS end
  local ex = {}
  if type(c.exclude) == "table" then
    for _, p in ipairs(c.exclude) do
      if type(p) == "string" and sub(p, 1, 1) == "/" and #p > 1 then ex[#ex + 1] = p end
    end
  end
  return { mode = mode, hours = hours, exclude = ex, pass_user = mode == "users" and c.pass_user == true }
end

-- ------------------------------------------------------------------ session cookie

function _M.make_session(hid, uid, ver, hours, hostname, now)
  local p = "s1." .. CURRENT.kid .. "." .. hid .. "." .. uid .. "." .. ver .. "." .. (now + hours * 3600)
  return p .. "." .. mac(CURRENT, p, hostname or "")
end

-- Returns { hid, uid, ver, exp } or nil, reason.
function _M.parse_session(tok, hid, hostname, now)
  if type(tok) ~= "string" or #tok > 200 then return nil, "malformed" end
  local kid, h, uid, ver, exp, sig = match(tok, "^s1%.(%x+)%.(%d+)%.(%d+)%.(%d+)%.(%d+)%.([%w_-]+)$")
  if not kid or #h > 18 or #uid > 18 or #ver > 18 or #exp > 12 then return nil, "malformed" end
  local key = KEYS and KEYS[kid]
  if not key then return nil, "unknown key" end
  local p = sub(tok, 1, #tok - #sig - 1)
  if not crypto.const_eq(mac(key, p, hostname or ""), sig) then return nil, "signature" end
  if tonumber(h) ~= tonumber(hid) then return nil, "host" end
  exp = tonumber(exp)
  if now >= exp then return nil, "expired" end
  return { hid = tonumber(h), uid = tonumber(uid), ver = tonumber(ver), exp = exp }
end

-- Set-Cookie value of a new session (hours: lifetime).
function _M.session_cookie(tok, hours, https)
  local c = _M.COOKIE .. "=" .. tok .. "; Path=/; Max-Age=" .. (hours * 3600) .. "; HttpOnly; SameSite=Lax"
  if https then c = c .. "; Secure" end
  return c
end

function _M.clear_cookie(https)
  local c = _M.COOKIE .. "=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax"
  if https then c = c .. "; Secure" end
  return c
end

-- ------------------------------------------------------------------ pushed state

-- corvus-control pushes (PUT /signin-sessions):
--   {"users": {"<id>": <version>}, "names": {"<id>": "<name>"},
--    "hosts": {"<host id>": <pin version>}, "access": {"<host id>": [<user id>, ...]}}
-- users: enabled users only; access: for hosts in users mode, the users of the allowed
-- groups. Returns lookup tables keyed by number, or nil for anything unusable.
function _M.normalize_state(raw)
  if type(raw) ~= "table" or type(raw.users) ~= "table" or type(raw.hosts) ~= "table" then return nil end
  local st = { users = {}, names = {}, hosts = {}, access = {} }
  for k, v in pairs(raw.users) do
    local id, ver = tonumber(k), tonumber(v)
    if id and ver then st.users[id] = ver end
  end
  if type(raw.names) == "table" then
    for k, v in pairs(raw.names) do
      local id = tonumber(k)
      if id and type(v) == "string" then st.names[id] = v end
    end
  end
  for k, v in pairs(raw.hosts) do
    local id, ver = tonumber(k), tonumber(v)
    if id and ver then st.hosts[id] = ver end
  end
  if type(raw.access) == "table" then
    for k, list in pairs(raw.access) do
      local id = tonumber(k)
      if id and type(list) == "table" then
        local set = {}
        for _, u in ipairs(list) do
          u = tonumber(u)
          if u then set[u] = true end
        end
        st.access[id] = set
      end
    end
  end
  return st
end

-- Whether a parsed session (parse_session) is still valid for host hid in mode, given the
-- pushed state (normalize_state). No state yet (nothing pushed): never valid.
function _M.session_ok(s, hid, mode, st)
  if not s or not st then return false end
  if mode == "pin" then
    return s.uid == 0 and st.hosts[hid] ~= nil and st.hosts[hid] == s.ver
  end
  if s.uid == 0 or st.users[s.uid] == nil or st.users[s.uid] ~= s.ver then return false end
  local a = st.access[hid]
  return a ~= nil and a[s.uid] == true
end

-- A user name as X-Corvus-User (names are [a-z0-9._@-], see validate.SigninUser).
function _M.header_name(name)
  if type(name) ~= "string" or #name == 0 or #name > 64 or find(name, "[^a-z0-9._@-]") then return nil end
  return name
end

-- ------------------------------------------------------------------ form token

local function form_bind(hid, hostname, nonce)
  return "form|" .. tostring(hid) .. "|" .. (hostname or "") .. "|" .. (nonce or "")
end

function _M.make_form(hid, hostname, nonce, now)
  local p = "k1." .. CURRENT.kid .. "." .. (now + _M.FORM_TTL)
  return p .. "." .. mac(CURRENT, p, form_bind(hid, hostname, nonce))
end

-- Returns true or false, reason. The nonce comes from the __corvus_lt cookie.
function _M.check_form(tok, hid, hostname, nonce, now)
  if type(tok) ~= "string" or #tok > 100 then return false, "malformed" end
  if type(nonce) ~= "string" or not find(nonce, "^%x+$") or #nonce ~= 32 then return false, "nonce" end
  local kid, exp, sig = match(tok, "^k1%.(%x+)%.(%d+)%.([%w_-]+)$")
  if not kid or #exp > 12 then return false, "malformed" end
  local key = KEYS and KEYS[kid]
  if not key then return false, "unknown key" end
  local p = sub(tok, 1, #tok - #sig - 1)
  if not crypto.const_eq(mac(key, p, form_bind(hid, hostname, nonce)), sig) then return false, "signature" end
  if now >= tonumber(exp) then return false, "expired" end
  return true
end

-- ------------------------------------------------------------------ request checks

-- Paths that never need a sign-in: the gateway's own endpoints and certificate validation.
function _M.exempt_path(uri)
  if type(uri) ~= "string" then return true end
  return sub(uri, 1, 10) == "/__corvus/" or sub(uri, 1, 28) == "/.well-known/acme-challenge/"
end

-- Whether the request is under an excluded path prefix. uri is nginx's normalised path
-- ($uri); raw is the request target as sent ($request_uri). The raw path must start with
-- the same prefix and must not hide separators or dot segments behind percent-encoding
-- or backslashes: a backend that decodes "%2F" or "\" itself could otherwise be reached
-- outside the excluded prefix ("/api%2F..%2Fadmin").
function _M.excluded(uri, raw, list)
  if type(list) ~= "table" or #list == 0 or type(uri) ~= "string" then return false end
  local hit
  for _, p in ipairs(list) do
    if sub(uri, 1, #p) == p then hit = p break end
  end
  if not hit then return false end
  if type(raw) ~= "string" then return false end
  local path = raw
  local q = find(raw, "?", 1, true)
  if q then path = sub(raw, 1, q - 1) end
  if sub(path, 1, #hit) ~= hit then return false end
  local l = lower(path)
  if find(l, "%2f", 1, true) or find(l, "%5c", 1, true) or find(l, "%2e", 1, true) or find(path, "\\", 1, true)
    or find(path, "/../", 1, true) or find(path, "/./", 1, true) then
    return false
  end
  return true
end

-- Where to go after signing in: a same-host path (challenge.safe_return rules), never
-- back to the gateway's own endpoints.
function _M.return_target(r, safe_return)
  r = safe_return(r)
  if sub(r, 1, 10) == "/__corvus/" then return "/" end
  return r
end

-- What an unauthenticated request gets: browsers asking for a page (GET/HEAD with an
-- Accept header that takes HTML) are sent to the login page; anything else (API clients,
-- form posts, scripts) gets 401 JSON.
function _M.wants_page(method, accept)
  if method ~= "GET" and method ~= "HEAD" then return false end
  if type(accept) == "table" then accept = accept[1] end
  return type(accept) == "string" and find(lower(accept), "text/html", 1, true) ~= nil
end

-- Removes the session cookie from a Cookie header value (the backend never sees it).
-- Returns the new value ("" when nothing is left).
function _M.strip_cookie(header)
  if type(header) == "table" then header = table.concat(header, "; ") end
  if type(header) ~= "string" then return "" end
  local out = {}
  for part in (header .. ";"):gmatch("([^;]*);") do
    local name = match(part, "^%s*([^=%s]+)")
    if name and name ~= _M.COOKIE then
      out[#out + 1] = match(part, "^%s*(.-)%s*$")
    end
  end
  return table.concat(out, "; ")
end

-- Values of the answer of corvus-control that go into the page, checked before use.
function _M.safe_qr(v)
  if type(v) ~= "string" or #v > 65536 or not find(v, "^data:image/png;base64,[A-Za-z0-9+/=]+$") then return "" end
  return v
end

function _M.safe_key(v)
  if type(v) ~= "string" or #v > 128 or not find(v, "^[A-Z2-7=]+$") then return "" end
  return v
end

function _M.safe_step(v)
  if type(v) ~= "string" or #v > 128 or not find(v, "^[%w_-]+$") then return "" end
  return v
end

-- $corvus_rule for an event corvus-control reports ("login_failed" → "login:failed").
local EVENTS = { succeeded = true, failed = true, limited = true, enroll = true, totp = true }
function _M.rule_tag(event)
  local e = type(event) == "string" and match(event, "^login_(%l+)$")
  if e and EVENTS[e] then return "login:" .. e end
  return nil
end

return _M
