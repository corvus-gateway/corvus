-- Corvus (Corvus) — waiting room enforcement in the OpenResty data plane.
--
-- Request path cost:
--   * valid admission token, younger than `refresh` s  → local HMAC check only (no Redis)
--   * older token                                      → 1 Redis heartbeat, token re-issued
--   * no/invalid token                                 → 1 Redis "enter" (admit or queue)
-- Redis failure follows the room's fail policy ("open" | "closed"); a holder of a
-- still-valid admission token is always let through.

local redis = require "resty.redis"
local cjson = require "cjson.safe"
local rstring = require "resty.string"
local rrandom = require "resty.random"
local crypto = require "corvus.crypto"

local ngx = ngx
local _M = {}

local NOT_FOUND, ACTIVE, WAITING, FULL = 0, 1, 2, 4
local PROMOTE_MAX = 200
local COOKIE_PREFIX = "corvus_"

local SECRET, DEBUG_KEY, REDIS_HOST, REDIS_PORT, REDIS_USER, REDIS_PASS
local hmac_key, CURRENT, KEYS   -- defined in the token section below
local SCRIPT, SCRIPT_SHA
local ROOMS = {}

local DEFAULTS = {
  capacity   = 500,   -- max concurrently admitted sessions
  active_ttl = 120,   -- s; an admitted session with no request for this long frees its slot
  grant_ttl  = 30,    -- s; time a promoted waiter has to come back and claim the slot
  queue_ttl  = 120,   -- s; a waiter that stops polling for this long loses its place
  refresh    = 15,    -- s; token age after which the gateway re-checks Redis (revocation lag)
  admit_rate = 0,     -- admissions per second (0 = unlimited, capacity only)
  fail       = "open",-- behaviour when Redis is unavailable: "open" | "closed"
  poll_min   = 3,     -- s; suggested poll interval near the front of the line
  poll_max   = 30,    -- s; suggested poll interval far back in the line
  max_queue  = 100000,-- waiters per room; beyond this new visitors get 503 (protects Redis memory)
  max_new_per_ip = 60,-- new queue entries per client IP per minute (0 = unlimited)
  bind_ip    = false, -- bind admission tokens to the client's network (/24, IPv6 /48)
}

-- ---------------------------------------------------------------- init

local function read_file(path)
  local f, err = io.open(path, "rb")
  if not f then error("corvus: cannot read " .. path .. ": " .. tostring(err)) end
  local data = f:read("*a")
  f:close()
  return data
end

-- policy_path: the policy.json of the configuration version being loaded
-- (falls back to $CORVUS_POLICY_FILE, then /etc/corvus/policy.json).
function _M.init(policy_path)
  SECRET = os.getenv("CORVUS_WR_SECRET")
  if not SECRET or #SECRET < 32 then
    error("corvus: CORVUS_WR_SECRET must be set (at least 32 characters)")
  end
  CURRENT = hmac_key(SECRET)
  KEYS = { [CURRENT.kid] = CURRENT }
  local prev = os.getenv("CORVUS_WR_SECRET_PREV")
  if prev and #prev >= 32 then
    local pk = hmac_key(prev)
    KEYS[pk.kid] = pk
  end
  DEBUG_KEY = os.getenv("CORVUS_DEBUG_KEY")
  if DEBUG_KEY == "" then DEBUG_KEY = nil end
  REDIS_HOST = os.getenv("CORVUS_REDIS_HOST") or "redis"
  REDIS_PORT = tonumber(os.getenv("CORVUS_REDIS_PORT") or "6379")
  REDIS_USER = os.getenv("CORVUS_REDIS_USER") or "corvus"
  REDIS_PASS = os.getenv("CORVUS_REDIS_PASSWORD")
  if REDIS_PASS == "" then REDIS_PASS = nil end

  SCRIPT = read_file(os.getenv("CORVUS_REDIS_SCRIPT") or "/opt/corvus/redis/waitroom.lua")
  SCRIPT_SHA = rstring.to_hex(ngx.sha1_bin(SCRIPT))

  local policy, err = cjson.decode(read_file(policy_path or os.getenv("CORVUS_POLICY_FILE") or "/etc/corvus/policy.json"))
  if not policy then error("corvus: invalid policy file: " .. tostring(err)) end
  for name, cfg in pairs(policy.waitrooms or {}) do
    if type(name) ~= "string" or #name > 32 or not name:match("^[a-z0-9][a-z0-9-]*$") then
      error("corvus: invalid waiting room name: " .. tostring(name))
    end
    local c = { name = name, cookie = name:gsub("-", "_") }
    for k, v in pairs(DEFAULTS) do
      if cfg[k] == nil then c[k] = v else c[k] = cfg[k] end
    end
    if c.fail ~= "open" and c.fail ~= "closed" then error("corvus: room " .. name .. ": fail must be open|closed") end
    ROOMS[name] = c
  end
end

-- ---------------------------------------------------------------- tokens

-- HMAC-SHA256 tokens: see corvus/crypto.lua (shared with the security-rule challenge).
local b64url, sha256_bin, hmac, const_eq = crypto.b64url, crypto.sha256_bin, crypto.hmac, crypto.const_eq
hmac_key = crypto.hmac_key

-- KEYS: kid → key (current + previous, for rotation); CURRENT signs new tokens.

-- Client network used for optional token binding: IPv4 /24, IPv6 first three groups (/48).
local function client_net()
  local ip = ngx.var.remote_addr or ""
  if ip:find(":", 1, true) then
    local a, b, c = ip:match("^(%x*):(%x*):(%x*)")
    return (a or "") .. ":" .. (b or "") .. ":" .. (c or "")
  end
  return ip:match("^(%d+%.%d+%.%d+)%.") or ip
end

-- token = v2.<kid>.<kind>.<room>.<sid>.<iat>.<exp>.<sig>   kind: a=admission, q=queue
-- `bind` (optional) is mixed into the MAC but not carried in the token, so a
-- bound token only verifies from the same client network.
local function make_token(kind, room, sid, iat, exp, bind)
  local p = "v2." .. CURRENT.kid .. "." .. kind .. "." .. room .. "." .. sid .. "." .. iat .. "." .. exp
  return p .. "." .. b64url(hmac(CURRENT, p .. (bind and ("|" .. bind) or "")))
end

local function parse_token(tok, kind, room, bind)
  if not tok or #tok > 240 then return nil end
  local kid, k, r, sid, iat, exp, sig = tok:match("^v2%.(%x+)%.(%l)%.([%l%d-]+)%.(%x+)%.(%d+)%.(%d+)%.([%w_-]+)$")
  if not kid or k ~= kind or r ~= room or #sid ~= 32 then return nil end
  local key = KEYS[kid]
  if not key then return nil end
  local p = "v2." .. kid .. "." .. k .. "." .. r .. "." .. sid .. "." .. iat .. "." .. exp
  if not const_eq(b64url(hmac(key, p .. (bind and ("|" .. bind) or ""))), sig) then return nil end
  exp = tonumber(exp)
  if ngx.time() >= exp then return nil end
  return { sid = sid, iat = tonumber(iat), exp = exp }
end

local function new_sid()
  local b = rrandom.bytes(16, true) or rrandom.bytes(16)
  return rstring.to_hex(b)
end

-- ---------------------------------------------------------------- cookies

local function add_cookie(name, value, max_age)
  local c = name .. "=" .. value .. "; Path=/; Max-Age=" .. max_age .. "; HttpOnly; SameSite=Lax"
  if ngx.var.scheme == "https" then c = c .. "; Secure" end
  local ctx = ngx.ctx
  ctx.corvus_cookies = ctx.corvus_cookies or {}
  table.insert(ctx.corvus_cookies, c)
end

local function issue_admission(room, sid)
  local now = ngx.time()
  local bind = room.bind_ip and client_net() or nil
  add_cookie(COOKIE_PREFIX .. "a_" .. room.cookie, make_token("a", room.name, sid, now, now + room.active_ttl, bind), room.active_ttl)
end

local function issue_queue(room, sid)
  local now = ngx.time()
  local ttl = 86400
  add_cookie(COOKIE_PREFIX .. "q_" .. room.cookie, make_token("q", room.name, sid, now, now + ttl), ttl)
end

local function clear_cookie(kind, room)
  add_cookie(COOKIE_PREFIX .. kind .. "_" .. room.cookie, "", 0)
end

local function admission_from_cookie(room)
  return parse_token(ngx.var["cookie_" .. COOKIE_PREFIX .. "a_" .. room.cookie], "a", room.name, room.bind_ip and client_net() or nil)
end

local function queue_from_cookie(room)
  return parse_token(ngx.var["cookie_" .. COOKIE_PREFIX .. "q_" .. room.cookie], "q", room.name)
end

-- header_filter_by_lua: merge our cookies with any Set-Cookie from the upstream.
function _M.header_filter()
  local cookies = ngx.ctx.corvus_cookies
  if not cookies then return end
  local existing = ngx.header["Set-Cookie"]
  if type(existing) == "string" then
    table.insert(cookies, 1, existing)
  elseif type(existing) == "table" then
    for i = #existing, 1, -1 do table.insert(cookies, 1, existing[i]) end
  end
  ngx.header["Set-Cookie"] = cookies
end

-- ---------------------------------------------------------------- redis

-- Circuit breaker: after a Redis failure, skip Redis for BREAKER_S seconds so
-- requests take the room's fail policy immediately instead of each waiting for
-- a connect timeout. Shared by all workers (lua_shared_dict corvus_wr_state).
local BREAKER_S = 3

-- Connect and authenticate (H12). Pooled connections are already authenticated,
-- so AUTH is sent only on a fresh connection.
local function redis_connect(red)
  local ok, err = red:connect(REDIS_HOST, REDIS_PORT)
  if not ok then return nil, err end
  if REDIS_PASS and red:get_reused_times() == 0 then
    ok, err = red:auth(REDIS_USER, REDIS_PASS)
    if not ok then
      red:close()
      return nil, "auth: " .. tostring(err)
    end
  end
  return true
end

local function breaker_open()
  local d = ngx.shared.corvus_wr_state
  return d and d:get("redis_down")
end

local function trip(err)
  local d = ngx.shared.corvus_wr_state
  if d and d:add("redis_down", true, BREAKER_S) then
    ngx.log(ngx.ERR, "corvus waitroom: redis unavailable, circuit open for ", BREAKER_S, "s: ", tostring(err))
  end
end

local function run(room, op, sid)
  if breaker_open() then return nil, "circuit open" end
  local red = redis:new()
  red:set_timeouts(200, 300, 300)
  local ok, err = redis_connect(red)
  if not ok then
    trip(err)
    return nil, "connect: " .. tostring(err)
  end
  local p = "corvus:wr:{" .. room.name .. "}:"
  local res
  res, err = red:evalsha(SCRIPT_SHA, 5,
    p .. "active", p .. "queue", p .. "seen", p .. "seq", p .. "rate",
    op, sid or "", room.capacity, room.active_ttl * 1000, room.grant_ttl * 1000,
    room.queue_ttl * 1000, room.admit_rate, PROMOTE_MAX, room.max_queue)
  if not res and err and err:find("NOSCRIPT", 1, true) then
    res, err = red:eval(SCRIPT, 5,
      p .. "active", p .. "queue", p .. "seen", p .. "seq", p .. "rate",
      op, sid or "", room.capacity, room.active_ttl * 1000, room.grant_ttl * 1000,
      room.queue_ttl * 1000, room.admit_rate, PROMOTE_MAX, room.max_queue)
  end
  if not res then
    -- Redis out of memory is overload, not an outage: answer "busy" instead of the
    -- fail-open/closed path, so filling Redis cannot switch a room to fail-open.
    if err and err:find("^OOM") then
      red:set_keepalive(10000, 512)
      ngx.log(ngx.ERR, "corvus waitroom ", room.name, ": redis out of memory")
      return { code = FULL, rank = 0, ticket = 0, waiting = 0, active = 0 }
    end
    red:close()
    -- Transport failures (timeout/closed) open the breaker; Redis error replies do not.
    if err and not err:find("^%u+ ") then trip(err) end
    return nil, err
  end
  red:set_keepalive(10000, 512)
  return { code = res[1], rank = res[2], ticket = res[3], waiting = res[4], active = res[5] }
end

-- ---------------------------------------------------------------- responses

local function poll_interval(room, rank)
  local s = room.poll_min + math.floor((rank or 0) / 500)
  if s > room.poll_max then s = room.poll_max end
  return s
end

local function send_json(status, body)
  ngx.status = status
  ngx.header["Content-Type"] = "application/json"
  ngx.header["Cache-Control"] = "no-store"
  ngx.print(cjson.encode(body))
  return ngx.exit(status)
end

local function wants_html()
  local m = ngx.req.get_method()
  if m ~= "GET" and m ~= "HEAD" then return false end
  local accept = ngx.var.http_accept or ""
  return accept:find("text/html", 1, true) ~= nil
end

local function send_waiting(room, res)
  local retry = poll_interval(room, res.rank)
  if wants_html() then
    ngx.header["Cache-Control"] = "no-store"
    return ngx.redirect("/__corvus/wait/" .. room.name .. "?r=" .. ngx.escape_uri(ngx.var.request_uri), 302)
  end
  ngx.header["Retry-After"] = tostring(retry)
  return send_json(503, {
    status = "waiting", room = room.name, rank = res.rank, ticket = res.ticket,
    waiting = res.waiting, retry_after = retry,
  })
end

local BUSY_HTML = [[<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width"><title>Corvus</title>]]
  .. [[<p style="font:16px system-ui;margin:20vh auto;max-width:28rem;text-align:center">]]
  .. [[대기 인원이 많아 지금은 대기열에 들어갈 수 없습니다. 잠시 후 다시 시도해 주세요.<br>]]
  .. [[<small>The queue is full right now. Please try again shortly.</small></p>]]

local function send_busy(room)
  ngx.ctx.corvus_wr = "full"
  ngx.header["Retry-After"] = "30"
  ngx.header["Cache-Control"] = "no-store"
  if wants_html() then
    ngx.status = 503
    ngx.header["Content-Type"] = "text/html; charset=utf-8"
    ngx.print(BUSY_HTML)
    return ngx.exit(503)
  end
  return send_json(503, { status = "full", room = room.name, retry_after = 30 })
end

-- Limits how many new queue entries one client IP may create per minute.
-- Returns false when over the limit; sids already seen are not counted again.
local function allow_new_entry(room, sid)
  local d = ngx.shared.corvus_wr_ip
  if not d or room.max_new_per_ip <= 0 then return true end
  if d:get("s:" .. sid) then return true end
  local n = d:incr("n:" .. room.name .. ":" .. (ngx.var.remote_addr or ""), 1, 0, 60)
  if n and n > room.max_new_per_ip then return false end
  d:set("s:" .. sid, true, room.queue_ttl + room.active_ttl)
  return true
end

-- Per-session request budget for the status/leave API: browsers poll every few
-- seconds, so more than 3 calls per second from one session is abuse. Bounds the
-- Redis work any single session can cause, independent of shared (NAT) IPs.
local function session_over_budget(sid)
  local d = ngx.shared.corvus_wr_ip
  if not d then return false end
  local n = d:incr("p:" .. sid, 1, 0, 1)
  return n and n > 3
end

local function send_too_many(room)
  ngx.ctx.corvus_wr = "ip-limit"
  ngx.header["Retry-After"] = "60"
  return send_json(429, { status = "rate_limited", room = room.name, retry_after = 60 })
end

local function fail(room, err)
  if err ~= "circuit open" then
    ngx.log(ngx.ERR, "corvus waitroom ", room.name, ": redis unavailable (", tostring(err), "), policy=fail-", room.fail)
  end
  ngx.ctx.corvus_wr = "fail-" .. room.fail
  if room.fail == "open" then return end
  if wants_html() then
    ngx.header["Cache-Control"] = "no-store"
    ngx.header["Retry-After"] = "10"
    ngx.status = 503
    ngx.header["Content-Type"] = "text/html; charset=utf-8"
    ngx.print("<!doctype html><meta charset=utf-8><title>Corvus</title><p>잠시 후 다시 시도해 주세요. / Please try again shortly.</p>")
    return ngx.exit(503)
  end
  ngx.header["Retry-After"] = "10"
  return send_json(503, { status = "unavailable", room = room.name, retry_after = 10 })
end

-- ---------------------------------------------------------------- access phase

-- access_by_lua: protect a location. Requires `set $corvus_room <name>;`.
function _M.access()
  local room = ROOMS[ngx.var.corvus_room or ""]
  if not room then
    ngx.log(ngx.ERR, "corvus: unknown waiting room '", tostring(ngx.var.corvus_room), "'")
    return ngx.exit(500)
  end

  local tok = admission_from_cookie(room)
  if tok and ngx.time() - tok.iat < room.refresh then
    ngx.ctx.corvus_wr = "pass"
    return -- fast path: no Redis
  end

  if tok then
    local res, err = run(room, "heartbeat", tok.sid)
    if not res then
      ngx.ctx.corvus_wr = "pass-stale"
      return -- valid token holders are never blocked by a Redis outage
    end
    if res.code == ACTIVE then
      issue_admission(room, tok.sid)
      ngx.ctx.corvus_wr = "refresh"
      return
    end
  end

  local q = queue_from_cookie(room)
  local sid = (q and q.sid) or (tok and tok.sid)
  if not sid then
    -- Cookie check: a first visit creates no server-side state. The client gets a
    -- signed queue cookie and must come back with it before Redis is touched, so
    -- cookie-less floods can neither fill the queue nor take admission slots.
    issue_queue(room, new_sid())
    ngx.ctx.corvus_wr = "cookie-check"
    ngx.header["Cache-Control"] = "no-store"
    if wants_html() then
      return ngx.redirect(ngx.var.request_uri, 302)
    end
    ngx.header["Retry-After"] = "1"
    return send_json(503, { status = "retry", room = room.name, retry_after = 1 })
  end
  if not allow_new_entry(room, sid) then return send_too_many(room) end
  local res, err = run(room, "enter", sid)
  if not res then return fail(room, err) end
  if res.code == FULL then return send_busy(room) end

  if res.code == ACTIVE then
    issue_admission(room, sid)
    if q then clear_cookie("q", room) end
    ngx.ctx.corvus_wr = "admit"
    return
  end

  if not q or q.sid ~= sid then issue_queue(room, sid) end
  ngx.ctx.corvus_wr = "queue"
  return send_waiting(room, res)
end

-- ---------------------------------------------------------------- control endpoints

-- content_by_lua for /__corvus/wr/<room>/<action>
function _M.endpoint()
  local name, action = ngx.var.uri:match("^/__corvus/wr/([a-z0-9-]+)/(%l+)$")
  local room = name and ROOMS[name]
  -- A host only serves the status API of the room it is attached to.
  local host_room = ngx.var.corvus_host_room
  if host_room and host_room ~= "" and host_room ~= name then room = nil end
  if not room then return send_json(404, { status = "unknown_room" }) end
  local method = ngx.req.get_method()

  if action == "status" and method == "GET" then
    local q = queue_from_cookie(room)
    local tok = admission_from_cookie(room)
    local sid = (q and q.sid) or (tok and tok.sid)
    if sid and session_over_budget(sid) then return send_too_many(room) end
    local res, err
    if q then
      res, err = run(room, "poll", q.sid)
      if res and res.code == NOT_FOUND then -- new or dropped → (re)join at the back
        if not allow_new_entry(room, q.sid) then return send_too_many(room) end
        res, err = run(room, "enter", q.sid)
      end
    elseif tok then
      res, err = run(room, "heartbeat", tok.sid)
      if res and res.code == NOT_FOUND then
        if not allow_new_entry(room, tok.sid) then return send_too_many(room) end
        res, err = run(room, "enter", tok.sid)
      end
    else
      -- No cookie yet: hand one out without touching Redis (see access()).
      issue_queue(room, new_sid())
      return send_json(200, { status = "waiting", room = room.name, rank = 0, ticket = 0, waiting = 0, pending = true, poll = 1 })
    end
    if res and res.code == FULL then return send_busy(room) end
    if not res then
      ngx.log(ngx.ERR, "corvus waitroom ", room.name, ": status failed: ", tostring(err))
      if room.fail == "open" then return send_json(200, { status = "active", room = room.name, degraded = true }) end
      return send_json(503, { status = "unavailable", room = room.name, retry_after = 10 })
    end
    if res.code == ACTIVE then
      issue_admission(room, (q and q.sid) or tok.sid)
      clear_cookie("q", room)
      return send_json(200, { status = "active", room = room.name, active = res.active, capacity = room.capacity })
    end
    return send_json(200, {
      status = "waiting", room = room.name, rank = res.rank, ticket = res.ticket,
      waiting = res.waiting, active = res.active, capacity = room.capacity,
      poll = poll_interval(room, res.rank),
    })
  end

  if action == "leave" and method == "POST" then
    local tok = admission_from_cookie(room)
    local q = queue_from_cookie(room)
    local sid = (tok and tok.sid) or (q and q.sid)
    if sid and session_over_budget(sid) then return send_too_many(room) end
    if sid then
      local res, err = run(room, "release", sid)
      if not res then return send_json(503, { status = "unavailable" }) end
    end
    clear_cookie("a", room)
    clear_cookie("q", room)
    return send_json(200, { status = "left", room = room.name })
  end

  local key = ngx.var.http_x_corvus_debug_key
  -- compare digests so neither the content nor the length of the key leaks via timing
  local debug_ok = DEBUG_KEY and key and const_eq(sha256_bin(key), sha256_bin(DEBUG_KEY))

  -- Test-only: wipe a room's state. Requires the debug key.
  if action == "reset" and method == "POST" then
    if not debug_ok then return send_json(403, { status = "forbidden" }) end
    local red = redis:new()
    red:set_timeouts(200, 300, 300)
    local ok, err = redis_connect(red)
    if not ok then return send_json(503, { status = "unavailable", error = tostring(err) }) end
    local p = "corvus:wr:{" .. room.name .. "}:"
    red:del(p .. "active", p .. "queue", p .. "seen", p .. "seq", p .. "rate")
    red:set_keepalive(10000, 512)
    return send_json(200, { status = "reset", room = room.name })
  end

  if action == "stats" and method == "GET" then
    if not debug_ok then return send_json(403, { status = "forbidden" }) end
    local res, err = run(room, "stats", "")
    if not res then return send_json(503, { status = "unavailable", error = tostring(err) }) end
    return send_json(200, {
      room = room.name, capacity = room.capacity, active = res.active, waiting = res.waiting,
      server_time_ms = math.floor(ngx.now() * 1000),
    })
  end

  return send_json(404, { status = "not_found" })
end

return _M
