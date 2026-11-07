-- Corvus security rules (Cloudflare-style custom rules), evaluated in the rewrite phase
-- of every proxy host — before rate limiting, Basic auth and the access queue.
--
-- One ordered list per host (rules.json, rendered by corvus-control). Walk top → bottom:
--   block → respond (blocked page / status) and stop
--   allow → skip later rules, attack-pattern blocking and rate limiting
--           (optionally Basic auth too) and stop
--   log   → record and continue
--   challenge → serve the challenge page (corvus/challenge.lua) and stop, unless the
--           request carries a valid challenge pass: then continue with later rules
-- After the rules, hosts with "block exploits" refuse requests that match the
-- attack-pattern map ($corvus_exploit), body inspection runs, and hosts in "under
-- attack" mode challenge every remaining visitor without a pass (corvus/underattack.lua).
-- Precedence: bans → block / allow / challenge rules → exploit and body blocking →
-- under-attack challenge → (later phases) rate limiting, Basic auth, access queue.
--
-- Expressions are in disjunctive normal form: any group whose conditions all match.

local cjson = require "cjson.safe"
local body = require "corvus.body"
local ualist = require "corvus.ualist"
local challenge = require "corvus.challenge"
local underattack = require "corvus.underattack"
local accesslog = require "corvus.accesslog"
local ipkey = require "corvus.ipkey"
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
local CHALLENGE_PAGE = [[<!doctype html><meta charset=utf-8><title>Checking your browser</title>]]
  .. [[<form method=post action="/__corvus/challenge/verify"><input type=hidden name=t value="{{TOKEN}}">]]
  .. [[<input type=hidden name=r value="{{RETURN}}"><button>Continue</button></form>]]

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

-- IPv4 networks are kept as masked 32-bit numbers (no string building per lookup).
local band, bor, lshift = bit.band, bit.bor, bit.lshift
local function v4mask(p) return p == 0 and 0 or lshift(-1, 32 - p) end
local function v4num(bin)
  local b1, b2, b3, b4 = byte(bin, 1, 4)
  return bor(lshift(b1, 24), lshift(b2, 16), lshift(b3, 8), b4)
end

local function ip_matcher(nets)
  local m = { [4] = { lens = {}, sets = {}, masks = {} }, [16] = { lens = {}, sets = {} } }
  for _, n in ipairs(nets) do
    local hex, p = n:match("^(%x+)/(%d+)$")
    p = tonumber(p)
    if hex and p and (#hex == 8 or #hex == 32) and p <= #hex * 4 then
      local fam = m[#hex / 2]
      local set = fam.sets[p]
      if not set then
        set = {}
        fam.sets[p] = set
        fam.lens[#fam.lens + 1] = p
        if fam.masks then fam.masks[p] = v4mask(p) end
      end
      if #hex == 8 then
        set[band(v4num(unhex(hex)), fam.masks[p])] = true
      else
        set[mask(unhex(hex), p)] = true
      end
    end
  end
  for _, fam in pairs(m) do table.sort(fam.lens, function(a, b) return a > b end) end
  return m
end

local function ip_in(m, bin)
  local fam = m[#bin]
  if not fam then return false end
  local lens, sets = fam.lens, fam.sets
  if #bin == 4 then
    local x, masks = v4num(bin), fam.masks
    for i = 1, #lens do
      local p = lens[i]
      if sets[p][band(x, masks[p])] then return true end
    end
    return false
  end
  for i = 1, #lens do
    local p = lens[i]
    if sets[p][mask(bin, p)] then return true end
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
      ch = r.action == "challenge" and challenge.settings(r.challenge) or nil,
    }
  end
  HOSTS = {}
  for hid, h in pairs(cfg.hosts or {}) do
    local list = {}
    for _, idx in ipairs(h.rules or {}) do list[#list + 1] = compiled[idx] end
    -- keys: proxy host id (number) or "r<id>" for redirection hosts
    local ua = underattack.settings(h.under_attack)
    HOSTS[tonumber(hid) or hid] = { rules = list, cf = h.cf and true or false, exploits = h.exploits and true or false,
      body = h.body or "off", body_max = tonumber(h.body_max) or 1048576,
      ua = ua, ua_rule = ua and { id = underattack.RULE, ch = ua.ch } or nil }
    local hc = HOSTS[tonumber(hid) or hid]
    hc.idle = #list == 0 and not hc.exploits and hc.body == "off" and not ua
  end
  GEO = load_geo(cfg.geo)
  if cfg.body_signatures then body.set_signatures(cfg.body_signatures) end
  if cfg.block_page then
    BLOCK_PAGE = read_file(cfg.block_page) or BLOCK_PAGE
  end
  if cfg.error_page then
    ERROR_PAGE = read_file(cfg.error_page) or ERROR_PAGE
  end
  if cfg.challenge_page then
    CHALLENGE_PAGE = read_file(cfg.challenge_page) or CHALLENGE_PAGE
  end
  challenge.init()
end

-- ------------------------------------------------------------------ under attack

local UA_V, UA_EX = nil, nil  -- per-worker exempt networks and their version

local function ua_exempt()
  local d = ngx.shared.corvus_ua
  if not d then return nil end
  local v = d:get("exempt_v")
  if not v then return nil end
  if v ~= UA_V then
    local list = cjson.decode(d:get("exempt") or "")
    UA_EX = ip_matcher(type(list) == "table" and list or {})
    UA_V = v
  end
  return UA_EX
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

-- Serves a block, error or challenge page: {{STATUS}} {{RID}} {{HOST}} {{IP}} {{TIME}}
-- (and the page's own `vars`) are replaced with HTML-escaped values; unknown
-- placeholders render empty.
local function send_page(tpl, status, vars)
  ngx.status = status
  ngx.header["Content-Type"] = "text/html; charset=utf-8"
  ngx.header["Cache-Control"] = "no-store"
  local page = string.gsub(tpl, "{{(%u+)}}", function(k)
    local v
    if vars and vars[k] ~= nil then v = vars[k]
    elseif k == "STATUS" then v = status
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

-- Challenge page. Status 403 (as Cloudflare's challenge pages): the request did not
-- reach the site, and unlike 503 (+ Retry-After) it neither invites HTTP clients to
-- retry in a loop nor shows up as a server error in 5xx monitoring and error rates.
-- Clients without a browser simply see a 403. Never cached (no-store); not counted
-- as bad behaviour (ngx.ctx.corvus_ch, see _M.log).
local function send_challenge(rule, ret)
  ngx.ctx.corvus_ch = true
  ngx.header["X-Corvus-Challenge"] = rule.ch.name
  ngx.header["X-Robots-Tag"] = "noindex"
  return send_page(CHALLENGE_PAGE, 403, challenge.page_vars(rule.id, rule.ch, ret))
end

local function challenge_rule(host, rid)
  if not host then return nil end
  if rid == underattack.RULE then return host.ua_rule end
  for _, rule in ipairs(host.rules) do
    if rule.id == rid and rule.ch then return rule end
  end
  return nil
end

local function is_acme(uri)  -- byte test first: no substring for ordinary paths
  return byte(uri, 2) == 46 and sub(uri, 1, 28) == "/.well-known/acme-challenge/"
end

-- A ban covers the exact address (manual bans) or the client key: the IPv4 address or
-- the IPv6 /64 (automatic bans), see ipkey.lua.
local function banned(bans, ip)
  if not ip then return false end
  if bans:get(ip) then return true end
  if find(ip, ":", 1, true) then
    local k = ipkey.client(ip)
    return k ~= ip and bans:get(k) ~= nil
  end
  return false
end

function _M.rewrite(host_id)
  local host = HOSTS[host_id]
  if not host then return end

  -- Banned client addresses (automatic or manual bans pushed by corvus-control).
  -- Certificate validation is never blocked.
  local bans = ngx.shared.corvus_bans
  if bans and banned(bans, ngx.var.remote_addr) then
    if is_acme(ngx.var.uri) then return end
    ngx.var.corvus_ban = "1"
    return deny(403)
  end
  -- Nothing else configured for this host (no rules, exploit or body blocking, under
  -- attack mode): done, without reading request fields.
  if host.idle then return end
  local uri = ngx.var.uri
  if is_acme(uri) then return end

  local ctx = {}
  for _, rule in ipairs(host.rules) do
    if matches(ctx, rule, host) then
      local a = rule.action
      if a == "log" then
        if ngx.var.corvus_rule == "" then ngx.var.corvus_rule = rule.tag end
      elseif a == "challenge" then
        -- the verify endpoint itself is never challenged (block rules still apply)
        if uri ~= challenge.VERIFY_PATH and not challenge.passed(ctx, rule.ch) then
          ngx.var.corvus_rule = rule.tag
          return send_challenge(rule, challenge.return_path())
        end
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
    -- the block page itself: a bare 403 would go through the host's error_page, which
    -- a rate limit policy answering 403 maps to its "rate limited" response
    return deny(403)
  end
  -- Request body inspection (virtual patching for body-borne exploits).
  if host.body ~= "off" then
    local sig = body.inspect(host.body_max)
    -- A compressed body cannot be inspected: hosts in block mode refuse it, hosts that
    -- only log record it as not inspected (it is not a detection).
    if sig == "encoded" and host.body ~= "block" then sig = "skip:encoded" end
    if sig then
      if sub(sig, 1, 5) == "skip:" then
        ngx.var.corvus_body = sig            -- not inspected (too large / unreadable): log only
      else
        ngx.var.corvus_body = sig .. ":" .. host.body
        if host.body == "block" then return deny(403) end
      end
    end
  end
  -- "Under attack" mode: challenge every visitor without a pass.
  if host.ua then
    local d = ngx.shared.corvus_ua
    local until_ = d and d:get("h:" .. tostring(host_id))
    local now = ngx.time()
    if underattack.active(host.ua, until_, now) then
      local ex, bin = ua_exempt(), ngx.var.binary_remote_addr
      local ip_ex = (ex ~= nil and bin ~= nil and ip_in(ex, bin)) or false
      if underattack.decide(host.ua, until_, now, uri, ip_ex, challenge.pass(ctx)) == "challenge" then
        ngx.var.corvus_rule = underattack.TAG .. ":challenge"
        return send_challenge(host.ua_rule, challenge.return_path())
      end
    end
  end
end

-- content_by_lua of /__corvus/challenge/verify (every proxy and redirection host):
-- POST t=<challenge token>&c=<counter>&r=<return path>. Solved → pass cookie and 303
-- back to r (same-host paths only); failed → a fresh challenge (403), or the block
-- page when the token does not name a challenge rule of this host.
-- Log field $corvus_rule: "<rule>:challenge_pass" / "<rule>:challenge_fail".
function _M.challenge_verify(host_id)
  ngx.ctx.corvus_ch = true
  if ngx.req.get_method() ~= "POST" then
    ngx.header["Allow"] = "POST"
    return ngx.exit(405)
  end
  ngx.req.read_body()
  local args = ngx.req.get_post_args(8)
  if type(args) ~= "table" then args = {} end
  local host = HOSTS[host_id]
  local ok, rid, res = challenge.verify(args, function(id) return challenge_rule(host, id) ~= nil end)
  local r = args.r
  if type(r) == "table" then r = r[1] end
  local ret = challenge.safe_return(r)
  local tag = rid == underattack.RULE and underattack.TAG or rid
  if ok then
    ngx.var.corvus_rule = tag .. ":challenge_pass"
    ngx.header["Set-Cookie"] = res
    ngx.header["Cache-Control"] = "no-store"
    return ngx.redirect(ret, 303)
  end
  local rule = rid and challenge_rule(host, rid)
  if not rule then return deny(403) end
  ngx.var.corvus_rule = tag .. ":challenge_fail"
  return send_challenge(rule, ret)
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

-- Status-port handler: replace the automatic under-attack activations. Body:
-- {"hosts": {"<proxy host id>": <activation ends, unix>}, "exempt": ["<hex net>/<prefix>", ...]}.
-- Entries expire on their own at the end of the activation, so a stopped
-- corvus-control never leaves a host challenged for longer than announced.
function _M.set_under_attack()
  ngx.req.read_body()
  local data = ngx.req.get_body_data()
  local cfg = data and cjson.decode(data)
  if type(cfg) ~= "table" or type(cfg.hosts) ~= "table" then
    ngx.status = 400
    ngx.say("bad request")
    return
  end
  local d = ngx.shared.corvus_ua
  if not d then
    ngx.status = 503
    ngx.say("no under-attack store")
    return
  end
  if type(cfg.exempt) == "table" then
    local raw = cjson.encode(cfg.exempt)
    if raw and d:get("exempt") ~= raw then
      d:set("exempt", raw)
      d:incr("exempt_v", 1, 0)
    end
  end
  local now = ngx.time()
  local keep, n = {}, 0
  for id, exp in pairs(cfg.hosts) do
    exp = tonumber(exp) or 0
    local key = "h:" .. tostring(id)
    if exp > now and d:set(key, exp, exp - now) then
      keep[key] = true
      n = n + 1
    end
  end
  for _, k in ipairs(d:get_keys(0)) do
    if sub(k, 1, 2) == "h:" and not keep[k] then d:delete(k) end
  end
  ngx.say(n)
end

-- ------------------------------------------------------------------ bad behaviour
--
-- BunkerWeb-style: responses with one of the configured status codes are counted per
-- client (IPv4 address or IPv6 /64) and proxy host (corvus_bb, TTL = window). When the
-- threshold is reached the client is banned here immediately (corvus_bans, first-ban duration) and a pending record
-- is queued (corvus_bb_q "p:<ip>"); corvus-control polls the records, stores the ban
-- with its escalation and acknowledges them with the next ban-list push.

local BB_V, BB_CFG = nil, nil  -- per-worker compiled settings and their version
local BB_AT = -1               -- when this worker last checked the version (ngx.now())

-- Settings pushed by corvus-control; each worker looks for a new version at most once a
-- second (a shared-dict read per request is measurable on the hot path).
local function bb_config()
  local now = ngx.now()
  if now - BB_AT < 1 then return BB_CFG end
  BB_AT = now
  local q = ngx.shared.corvus_bb_q
  if not q then return nil end
  local v = q:get("cfg_v")
  if not v then
    BB_V, BB_CFG = nil, nil
    return nil
  end
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

-- Access queue decisions that answer the client instead of the site (waitroom.lua).
local WR_HELD = { ["cookie-check"] = true, queue = true, full = true }

-- log_by_lua of every proxy host: access log record, then bad-behaviour counting. room: the host has an access queue (its decision is
-- in ngx.ctx; ngx.ctx is not touched otherwise, creating it costs a table per request).
function _M.log(host_id, room)
  if room then
    local wr = ngx.ctx.corvus_wr
    if wr then ngx.var.corvus_wr = wr end
  end
  accesslog.record()  -- the host's access log (nginx's own access_log is off there)
  local cfg = bb_config()
  if not cfg then return end
  local status = tonumber(ngx.var.status)
  if not status or not cfg.codes[status] then return end
  -- already banned, allowed by a security rule, answered with a challenge or held by the
  -- access queue (cookie check, waiting, queue full): not counted
  if ngx.var.corvus_ban == "1" or ngx.var.corvus_skip == "1" or ngx.ctx.corvus_ch then return end
  if room and WR_HELD[ngx.ctx.corvus_wr] then return end
  local uri = ngx.var.uri or ""
  -- the gateway's own endpoints (certificate validation, access queue) never count
  if sub(uri, 1, 28) == "/.well-known/acme-challenge/" or sub(uri, 1, 10) == "/__corvus/" then return end
  local bin = ngx.var.binary_remote_addr
  if not bin or ip_in(cfg.exempt, bin) then return end
  -- counted and banned per client key: an IPv6 host can switch addresses inside its /64
  local ip = ipkey.client(ngx.var.remote_addr)
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
