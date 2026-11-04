-- Corvus security rules (Cloudflare-style custom rules), evaluated in the rewrite phase
-- of every proxy host — before rate limiting, Basic auth and the access queue.
--
-- One ordered list per host (rules.json, rendered by corvus-control). Walk top → bottom:
--   block → respond (blocked page / status) and stop
--   allow → skip later rules, attack-pattern blocking and rate limiting
--           (optionally Basic auth too) and stop
--   log   → record and continue
-- After the rules, hosts with "block exploits" refuse requests that match the
-- attack-pattern map ($corvus_exploit).
--
-- Expressions are in disjunctive normal form: any group whose conditions all match.

local cjson = require "cjson.safe"
local body = require "corvus.body"
local ualist = require "corvus.ualist"
local ffi = require "ffi"
local bit = require "bit"

local find, sub, lower, byte = string.find, string.sub, string.lower, string.byte
local re_find = ngx.re.find
local tonumber, type, pairs, ipairs = tonumber, type, pairs, ipairs

local _M = {}

local HOSTS = {}      -- host id → { rules = {...}, cf = bool, exploits = bool }
local GEO             -- country range table (see control/internal/geoip)
local BLOCK_PAGE = "<!doctype html><title>Blocked</title><p>Access denied.</p>"
local ERROR_PAGE = "<!doctype html><title>Unavailable</title><p>The site is temporarily unavailable. Please try again shortly.</p>"

pcall(ffi.cdef, "int memcmp(const void *s1, const void *s2, size_t n);")

local function read_file(path)
  local f, err = io.open(path, "rb")
  if not f then return nil, err end
  local d = f:read("*a")
  f:close()
  return d
end

-- ------------------------------------------------------------------ IP matching

-- Networks arrive pre-encoded by the compiler as "<hex network bytes>/<prefix>"
-- (8 hex digits for IPv4, 32 for IPv6). A matcher keeps, per address family and
-- prefix length, a set of masked network byte strings: lookup cost is one table
-- probe per distinct prefix length, independent of the number of networks.
local function unhex(h)
  return (h:gsub("..", function(x) return string.char(tonumber(x, 16)) end))
end

local function mask(bin, p)
  local full = math.floor(p / 8)
  local s = sub(bin, 1, full)
  local rem = p % 8
  if rem > 0 then
    local b = byte(bin, full + 1)
    local m = 256 - 2 ^ (8 - rem)
    s = s .. string.char(bit.band(b, m))
  end
  return s
end

local function ip_matcher(nets)
  local m = { [4] = { lens = {}, sets = {} }, [16] = { lens = {}, sets = {} } }
  for _, n in ipairs(nets) do
    local hex, p = n:match("^(%x+)/(%d+)$")
    p = tonumber(p)
    if hex and p and (#hex == 8 or #hex == 32) then
      local fam = m[#hex / 2]
      local set = fam.sets[p]
      if not set then
        set = {}
        fam.sets[p] = set
        fam.lens[#fam.lens + 1] = p
      end
      set[mask(unhex(hex), p)] = true
    end
  end
  for _, fam in pairs(m) do table.sort(fam.lens, function(a, b) return a > b end) end
  return m
end

local function ip_in(m, bin)
  local fam = m[#bin]
  if not fam then return false end
  for _, p in ipairs(fam.lens) do
    if fam.sets[p][mask(bin, p)] then return true end
  end
  return false
end

-- ------------------------------------------------------------------ country

local function load_geo(path)
  if not path or path == "" then return nil end
  local data = read_file(path)
  if not data or #data < 16 or sub(data, 1, 6) ~= "CVGEO1" then return nil end
  local p = ffi.cast("const uint8_t*", data)
  local hdr = ffi.cast("const uint32_t*", p + 8)
  local n4, n6 = tonumber(hdr[0]), tonumber(hdr[1])
  if #data ~= 16 + n4 * 6 + n6 * 18 or n4 == 0 then
    ngx.log(ngx.ERR, "corvus rules: country database has an unexpected size, ignored")
    return nil
  end
  return {
    data = data,  -- keeps the buffer alive for the pointers below
    n4 = n4, n6 = n6,
    s4 = ffi.cast("const uint32_t*", p + 16),
    c4 = 16 + n4 * 4,
    s6 = p + 16 + n4 * 6,
    c6 = 16 + n4 * 6 + n6 * 16,
  }
end

local function geo_lookup(bin)
  local g = GEO
  if not g then return nil end
  local lo, hi = 0, -1
  if #bin == 4 then
    local b1, b2, b3, b4 = byte(bin, 1, 4)
    local x = ((b1 * 256 + b2) * 256 + b3) * 256 + b4
    lo, hi = 0, g.n4 - 1
    while lo < hi do   -- last index with start <= x
      local mid = math.floor((lo + hi + 1) / 2)
      if tonumber(g.s4[mid]) <= x then lo = mid else hi = mid - 1 end
    end
    local o = g.c4 + lo * 2
    return sub(g.data, o + 1, o + 2)
  elseif #bin == 16 and g.n6 > 0 then
    local key = ffi.cast("const uint8_t*", bin)
    lo, hi = 0, g.n6 - 1
    while lo < hi do
      local mid = math.floor((lo + hi + 1) / 2)
      if ffi.C.memcmp(g.s6 + mid * 16, key, 16) <= 0 then lo = mid else hi = mid - 1 end
    end
    local o = g.c6 + lo * 2
    return sub(g.data, o + 1, o + 2)
  end
  return nil
end

-- ------------------------------------------------------------------ compile

local function compile_cond(c)
  local cc = { f = c.f, op = c.op, ci = c.ci and true or false }
  if c.f == "http.header" then cc.var = "http_" .. c.n:gsub("-", "_") end
  local v = c.v or {}
  if c.f == "ip.src" then
    cc.ip = ip_matcher(v)
  elseif c.f == "http.user_agent_list" then
    cc.ua = ualist.new(v)
  elseif c.op == "in" or c.op == "not_in" then
    cc.set = {}
    for _, x in ipairs(v) do cc.set[cc.ci and lower(x) or x] = true end
  elseif c.op == "matches" or c.op == "not_matches" then
    cc.re, cc.reopt = v[1], cc.ci and "joi" or "jo"
    local ok, err = pcall(re_find, "", cc.re, cc.reopt)
    if not ok or err == false then
      ngx.log(ngx.ERR, "corvus rules: invalid regex ignored: ", tostring(err))
      cc.re = nil
    end
  else
    cc.v = cc.ci and lower(v[1] or "") or (v[1] or "")
  end
  return cc
end

function _M.init(path)
  local raw = read_file(path)
  if not raw then return end
  local cfg = cjson.decode(raw)
  if type(cfg) ~= "table" then
    ngx.log(ngx.ERR, "corvus rules: cannot parse ", path)
    return
  end
  local compiled = {}
  for i, r in ipairs(cfg.rules or {}) do
    local groups = {}
    for gi, g in ipairs(r.groups or {}) do
      local cs = {}
      for ci, c in ipairs(g) do cs[ci] = compile_cond(c) end
      groups[gi] = cs
    end
    compiled[i] = {
      id = r.id, action = r.action, status = r.status or 403, skip_auth = r.skip_auth and true or false,
      tag = r.id .. ":" .. r.action, groups = groups,
    }
  end
  HOSTS = {}
  for hid, h in pairs(cfg.hosts or {}) do
    local list = {}
    for _, idx in ipairs(h.rules or {}) do list[#list + 1] = compiled[idx] end
    -- keys: proxy host id (number) or "r<id>" for redirection hosts
    HOSTS[tonumber(hid) or hid] = { rules = list, cf = h.cf and true or false, exploits = h.exploits and true or false,
      body = h.body or "off", body_max = tonumber(h.body_max) or 1048576 }
  end
  GEO = load_geo(cfg.geo)
  if cfg.body_signatures then body.set_signatures(cfg.body_signatures) end
  if cfg.block_page then
    BLOCK_PAGE = read_file(cfg.block_page) or BLOCK_PAGE
  end
  if cfg.error_page then
    ERROR_PAGE = read_file(cfg.error_page) or ERROR_PAGE
  end
end

-- ------------------------------------------------------------------ evaluate

-- Request fields, read lazily and cached per request.
local function field(ctx, c, host)
  local f = c.f
  local key = c.var or f
  local v = ctx[key]
  if v ~= nil then
    if v == false then return nil end
    return v
  end
  if f == "ip.src" then
    v = ngx.var.binary_remote_addr
  elseif f == "ip.country" then
    -- Cloudflare's country header is trusted only when this host takes the client
    -- address from Cloudflare and real-IP processing actually replaced the peer address.
    if host.cf and ngx.var.realip_remote_addr ~= ngx.var.remote_addr then
      local h = ngx.var.http_cf_ipcountry
      if h and find(h, "^%u%u$") then v = h end
    end
    if not v then v = geo_lookup(ngx.var.binary_remote_addr or "") end
    if v then ngx.var.corvus_cc = v end
  elseif f == "http.host" then v = ngx.var.host
  elseif f == "http.path" then v = ngx.var.uri
  elseif f == "http.query" then v = ngx.var.args or ""
  elseif f == "http.method" then v = ngx.req.get_method()
  elseif f == "http.scheme" then v = ngx.var.scheme
  elseif f == "http.user_agent" then v = ngx.var.http_user_agent
  elseif f == "http.user_agent_list" then  -- lower-cased once for all list conditions
    v = ngx.var.http_user_agent
    if type(v) == "table" then v = v[1] end
    if v then v = lower(v) end
  elseif f == "http.referer" then v = ngx.var.http_referer
  elseif f == "http.header" then v = ngx.var[c.var]
  elseif f == "http.body" then v = body.text(host.body_max)  -- normalised (see corvus/body.lua)
  end
  if type(v) == "table" then v = v[1] end
  ctx[key] = (v == nil) and false or v
  return v
end

local function test(ctx, c, host)
  local op = c.op
  local v = field(ctx, c, host)
  if op == "exists" then return v ~= nil and v ~= "" end
  if op == "not_exists" then return v == nil or v == "" end
  if c.f == "ip.src" then
    local hit = v ~= nil and ip_in(c.ip, v)
    if op == "in" then return hit end
    return not hit
  end
  if c.f == "http.user_agent_list" then
    local hit = ualist.match(c.ua, v)  -- absent User-Agent: never "in" a list
    if op == "in" then return hit end
    return not hit
  end
  if c.f == "ip.country" and v == nil then
    return false  -- no country information: never match (avoid blocking everyone)
  end
  if v == nil then
    -- absent header/value: only negative operators match
    return op == "ne" or op == "not_contains" or op == "not_matches" or op == "not_in"
  end
  if c.ci then v = lower(v) end
  if op == "in" then return c.set[v] == true end
  if op == "not_in" then return c.set[v] ~= true end
  if op == "eq" then return v == c.v end
  if op == "ne" then return v ~= c.v end
  if op == "contains" then return find(v, c.v, 1, true) ~= nil end
  if op == "not_contains" then return find(v, c.v, 1, true) == nil end
  if op == "starts_with" then return sub(v, 1, #c.v) == c.v end
  if op == "ends_with" then return #c.v <= #v and sub(v, -#c.v) == c.v end
  if op == "matches" or op == "not_matches" then
    if not c.re then return false end
    local from = re_find(v, c.re, c.reopt)  -- errors (e.g. match limit) count as no match
    if op == "matches" then return from ~= nil end
    return from == nil
  end
  return false
end

local function matches(ctx, rule, host)
  for _, g in ipairs(rule.groups) do
    local all = true
    for _, c in ipairs(g) do
      if not test(ctx, c, host) then all = false break end
    end
    if all then return true end
  end
  return false
end

local HTML_ESC = { ["&"] = "&amp;", ["<"] = "&lt;", [">"] = "&gt;", ['"'] = "&quot;", ["'"] = "&#39;" }
local function esc(s)
  return (string.gsub(tostring(s or ""), "[&<>\"']", HTML_ESC))
end

-- Serves a block or error page: {{STATUS}} {{RID}} {{HOST}} {{IP}} {{TIME}} are
-- replaced with HTML-escaped values; unknown placeholders render empty.
local function send_page(tpl, status)
  ngx.status = status
  ngx.header["Content-Type"] = "text/html; charset=utf-8"
  ngx.header["Cache-Control"] = "no-store"
  local page = string.gsub(tpl, "{{(%u+)}}", function(k)
    local v
    if k == "STATUS" then v = status
    elseif k == "RID" then v = ngx.var.request_id or "-"
    elseif k == "HOST" then v = ngx.var.host
    elseif k == "IP" then v = ngx.var.remote_addr
    elseif k == "TIME" then v = ngx.http_time(ngx.time())
    end
    return esc(v)
  end)
  ngx.print(page)
  return ngx.exit(ngx.HTTP_OK)  -- status already set; finalize the request
end

local function deny(status)
  if status == 444 then return ngx.exit(444) end
  return send_page(BLOCK_PAGE, status)
end

-- content_by_lua of the per-host error_page locations (@corvus_e502 …): only
-- errors nginx generates itself (upstream refused / timed out / invalid response).
function _M.upstream_error(status)
  return send_page(ERROR_PAGE, status)
end

function _M.rewrite(host_id)
  local host = HOSTS[host_id]
  if not host then return end
  local uri = ngx.var.uri
  if sub(uri, 1, 28) == "/.well-known/acme-challenge/" then return end  -- never block certificate validation

  -- Banned client addresses (automatic or manual bans pushed by corvus-control).
  local bans = ngx.shared.corvus_bans
  if bans and bans:get(ngx.var.remote_addr) then
    ngx.var.corvus_ban = "1"
    return deny(403)
  end

  local ctx = {}
  for _, rule in ipairs(host.rules) do
    if matches(ctx, rule, host) then
      local a = rule.action
      if a == "log" then
        if ngx.var.corvus_rule == "" then ngx.var.corvus_rule = rule.tag end
      else
        ngx.var.corvus_rule = rule.tag
        if a == "block" then
          return deny(rule.status)
        end
        -- allow: skip later rules, attack-pattern blocking and rate limiting
        ngx.var.corvus_skip = "1"
        if rule.skip_auth then ngx.var.corvus_auth = "off" end
        return
      end
    end
  end
  if host.exploits and ngx.var.corvus_exploit ~= "" then
    return ngx.exit(403)
  end
  -- Request body inspection (virtual patching for body-borne exploits).
  if host.body ~= "off" then
    local sig = body.inspect(host.body_max)
    if sig then
      if sub(sig, 1, 5) == "skip:" then
        ngx.var.corvus_body = sig            -- not inspected (too large / unreadable): log only
      else
        ngx.var.corvus_body = sig .. ":" .. host.body
        if host.body == "block" then return deny(403) end
      end
    end
  end
end

-- Status-port handler: replace the ban list. Body: {"ips": {"<ip>": <expires unix | 0>}}.
-- Entries are written with a fresh generation tag first, then stale ones are removed, so
-- a ban never lapses while the list is being replaced.
function _M.set_bans()
  ngx.req.read_body()
  local data = ngx.req.get_body_data()
  local cfg = data and cjson.decode(data)
  if type(cfg) ~= "table" or type(cfg.ips) ~= "table" then
    ngx.status = 400
    ngx.say("bad request")
    return
  end
  local d = ngx.shared.corvus_bans
  if not d then
    ngx.status = 503
    ngx.say("no ban store")
    return
  end
  local now = ngx.time()
  local q = ngx.shared.corvus_bb_q
  if q then
    -- bad-behaviour settings (applied by every worker on its next request)
    if type(cfg.bb) == "table" then
      local raw = cjson.encode(cfg.bb)
      if raw and q:get("cfg") ~= raw then
        q:set("cfg", raw)
        q:incr("cfg_v", 1, 0)
      end
    end
    -- gateway bans corvus-control has processed: drop their pending records
    if type(cfg.ack) == "table" then
      for ip, id in pairs(cfg.ack) do
        local key = "p:" .. tostring(ip)
        local h = cjson.decode(q:get(key) or "")
        if type(h) == "table" and h.id == id then q:delete(key) end
      end
    end
  end
  local gen = tostring(now) .. "-" .. tostring(math.random(1, 1000000000))
  local n = 0
  for ip, exp in pairs(cfg.ips) do
    exp = tonumber(exp) or 0
    local ttl = exp > 0 and (exp - now) or 0
    if exp == 0 or ttl > 0 then
      local ok = d:set(ip, gen, ttl)
      if ok then n = n + 1 end
    end
  end
  for _, k in ipairs(d:get_keys(0)) do
    -- keep gateway-local bad-behaviour bans corvus-control has not recorded yet
    if d:get(k) ~= gen and not (q and q:get("p:" .. k)) then d:delete(k) end
  end
  ngx.say(n)
end

-- ------------------------------------------------------------------ bad behaviour
--
-- BunkerWeb-style: responses with one of the configured status codes are counted per
-- client IP and proxy host (corvus_bb, TTL = window). When the threshold is reached the
-- IP is banned here immediately (corvus_bans, first-ban duration) and a pending record
-- is queued (corvus_bb_q "p:<ip>"); corvus-control polls the records, stores the ban
-- with its escalation and acknowledges them with the next ban-list push.

local BB_V, BB_CFG = nil, nil  -- per-worker compiled settings and their version

local function bb_config()
  local q = ngx.shared.corvus_bb_q
  if not q then return nil end
  local v = q:get("cfg_v")
  if not v then return nil end
  if v ~= BB_V then
    local c = cjson.decode(q:get("cfg") or "")
    local cfg
    if type(c) == "table" and c.enabled == true and type(c.codes) == "table" then
      local codes, list = {}, {}
      for _, x in ipairs(c.codes) do
        x = tonumber(x)
        if x then codes[x] = true; list[#list + 1] = x end
      end
      cfg = {
        codes = codes, list = list,
        threshold = math.max(tonumber(c.threshold) or 10, 1),
        window = math.max(tonumber(c.window) or 60, 1),
        duration = math.max(tonumber(c.duration) or 900, 1),
        exempt = ip_matcher(type(c.exempt) == "table" and c.exempt or {}),
      }
    end
    BB_V, BB_CFG = v, cfg
  end
  return BB_CFG
end

-- log_by_lua of every proxy host.
function _M.log(host_id)
  if ngx.ctx.corvus_wr then ngx.var.corvus_wr = ngx.ctx.corvus_wr end
  local cfg = bb_config()
  if not cfg then return end
  local status = tonumber(ngx.var.status)
  if not status or not cfg.codes[status] then return end
  -- already banned, or allowed by a security rule: not counted
  if ngx.var.corvus_ban == "1" or ngx.var.corvus_skip == "1" then return end
  local uri = ngx.var.uri or ""
  -- the gateway's own endpoints (certificate validation, access queue) never count
  if sub(uri, 1, 28) == "/.well-known/acme-challenge/" or sub(uri, 1, 10) == "/__corvus/" then return end
  local bin = ngx.var.binary_remote_addr
  if not bin or ip_in(cfg.exempt, bin) then return end
  local ip = ngx.var.remote_addr
  local bans, d, q = ngx.shared.corvus_bans, ngx.shared.corvus_bb, ngx.shared.corvus_bb_q
  if not bans or not d or not q or bans:get(ip) then return end

  local base = ip .. "|" .. tostring(host_id)
  local n = d:incr(base, 1, 0, cfg.window)
  d:incr(base .. "|" .. status, 1, 0, cfg.window)
  if not n or n < cfg.threshold then return end

  local counts = {}
  for _, code in ipairs(cfg.list) do
    local k = base .. "|" .. code
    local c = d:get(k)
    if c then counts[tostring(code)] = c; d:delete(k) end
  end
  d:delete(base)
  local id = tostring(ngx.now()) .. "-" .. tostring(math.random(1, 1000000000))
  local rec = cjson.encode({ id = id, ip = ip, host = ngx.var.corvus_host, counts = counts,
    window = cfg.window, t = ngx.time() })
  -- pending record first: a concurrent ban-list push then keeps the local ban
  q:set("p:" .. ip, rec, cfg.duration)
  bans:set(ip, "bb", cfg.duration)
  ngx.log(ngx.WARN, "corvus: bad behaviour ban ", ip, " on ", ngx.var.corvus_host,
    " (", n, " responses in ", cfg.window, "s)")
end

-- Status-port handler: pending gateway bans, {"hits": [<record>, ...]}.
function _M.bad_behaviour_hits()
  local q = ngx.shared.corvus_bb_q
  local out = {}
  if q then
    for _, k in ipairs(q:get_keys(0)) do
      if sub(k, 1, 2) == "p:" then
        local v = q:get(k)
        if v then out[#out + 1] = v end
      end
    end
  end
  ngx.say('{"hits":[', table.concat(out, ","), ']}')
end

return _M
