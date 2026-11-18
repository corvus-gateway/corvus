-- Unit tests for the ban-list push handler (set_bans) and the pending bad-behaviour
-- bans handler in corvus/rules.lua. Run with plain LuaJIT from the repository root
-- (CI lua job):  luajit gateway/test/bans_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local eq = T.eq

-- JSON stand-in: request bodies and pending records are tokens for prepared tables
local JSON = {}
package.loaded["cjson.safe"] = {
  decode = function(s) return JSON[s] end,
  encode = function(v) if type(v) == "string" then return '"' .. v .. '"' end return "{}" end,
}
for _, m in ipairs({ "corvus.body", "corvus.ualist", "corvus.challenge",
  "corvus.underattack", "corvus.accesslog", "corvus.ipkey" }) do
  package.loaded[m] = {}
end
ngx.re = { find = function() return nil end }

-- shared dict with TTLs (ngx.time), counting writes and full-key sweeps
local function dict()
  local d = { data = {}, exp = {}, sets = 0, sweeps = 0 }
  function d:get(k)
    local e = self.exp[k]
    if e and e ~= 0 and e <= ngx.time() then return nil end
    return self.data[k]
  end
  function d:set(k, v, ttl)
    self.sets = self.sets + 1
    self.data[k], self.exp[k] = v, (ttl and ttl > 0) and (ngx.time() + ttl) or 0
    return true
  end
  function d:add(k, v, ttl)
    if self:get(k) ~= nil then return false, "exists" end
    return self:set(k, v, ttl)
  end
  function d:delete(k) self.data[k], self.exp[k] = nil, nil end
  function d:get_keys()
    self.sweeps = self.sweeps + 1
    local out = {}
    for k in pairs(self.data) do
      if self:get(k) ~= nil then out[#out + 1] = k end
    end
    return out
  end
  function d:incr(k, by, init)
    local v = (self:get(k) or init) + by
    self.data[k] = v
    return v
  end
  return d
end

local out
local body
ngx.req = { read_body = function() end, get_body_data = function() return body end }
ngx.say = function(...) out = table.concat({ ... }) end
ngx.now = function() return ngx.time() + 0.25 end

-- the per-start status key (CORVUS_STATUS_KEY) and the header corvus-control sends
local STATUS_KEY = "k3y-0123456789abcdef"
local real_getenv = os.getenv
os.getenv = function(k)
  if k == "CORVUS_STATUS_KEY" then return STATUS_KEY end
  return real_getenv(k)
end
ngx.var.http_x_corvus_status_key = STATUS_KEY

local rules = require("corvus.rules")

local now = 1800000000
T.set_time(now)
local bans, q = dict(), dict()
ngx.shared = { corvus_bans = bans, corvus_bb_q = q }

local function push(cfg)
  body = "body" .. tostring(cfg)
  JSON[body] = cfg
  out, ngx.status = nil, 200
  rules.set_bans()
  return out
end

-- state before the first push: a stale control ban, a gateway-local bad-behaviour ban
-- corvus-control has not recorded yet (pending record), and one whose record is gone
bans:set("192.0.2.50", tostring(now + 300), 300)
bans:set("192.0.2.77", "bb", 900)
q:set("p:192.0.2.77", "rec77", 900)
JSON["rec77"] = { id = "h77", ip = "192.0.2.77" }
bans:set("192.0.2.78", "bb", 900)

-- full push (no "removed"): the whole store is swept
eq(push({ ips = { ["203.0.113.1"] = 0, ["203.0.113.2"] = now + 600, ["198.51.100.9"] = now - 5 } }), "2", "full push: entries set")
eq(bans:get("203.0.113.1"), "0", "permanent ban")
eq(bans:get("203.0.113.2"), tostring(now + 600), "ban with expiry")
eq(bans.exp["203.0.113.2"], now + 600, "ban expires with its end")
eq(bans:get("198.51.100.9"), nil, "ended ban not written")
eq(bans:get("192.0.2.50"), nil, "stale ban removed by the sweep")
eq(bans:get("192.0.2.77"), "bb", "pending bad-behaviour ban kept")
eq(bans:get("192.0.2.78"), nil, "local ban without pending record removed")
eq(bans.sweeps, 1, "full push sweeps once")

-- the same list again with "removed": nothing written, no sweep
local sets = bans.sets
eq(push({ ips = { ["203.0.113.1"] = 0, ["203.0.113.2"] = now + 600 }, removed = {} }), "2", "unchanged push")
eq(bans.sets, sets, "unchanged entries are not rewritten")
eq(bans.sweeps, 1, "delta push does not sweep")

-- delta: a lifted ban is deleted; a pending local ban listed as removed is kept
push({ ips = { ["203.0.113.1"] = 0 }, removed = { "203.0.113.2", "192.0.2.77" } })
eq(bans:get("203.0.113.2"), nil, "removed ban deleted")
eq(bans:get("203.0.113.1"), "0", "remaining ban kept")
eq(bans:get("192.0.2.77"), "bb", "pending local ban survives a delta push")
eq(bans.sweeps, 1, "still no sweep")

-- a changed expiry is written
push({ ips = { ["203.0.113.1"] = now + 60 }, removed = {} })
eq(bans:get("203.0.113.1"), tostring(now + 60), "expiry updated")
eq(bans.exp["203.0.113.1"], now + 60, "TTL updated")

-- acknowledgements: the pending record goes; the local ban goes too unless
-- corvus-control keeps it (then it carries corvus-control's expiry)
bans:set("192.0.2.88", "bb", 900)
q:set("p:192.0.2.88", "rec88", 900)
JSON["rec88"] = { id = "h88" }
push({ ips = { ["203.0.113.1"] = now + 60, ["192.0.2.88"] = now + 1800 }, removed = {},
  ack = { ["192.0.2.77"] = "h77", ["192.0.2.88"] = "h88" } })
eq(q:get("p:192.0.2.77"), nil, "acknowledged record dropped")
eq(bans:get("192.0.2.77"), nil, "acknowledged ban not kept by corvus-control removed")
eq(q:get("p:192.0.2.88"), nil, "acknowledged record dropped (kept ban)")
eq(bans:get("192.0.2.88"), tostring(now + 1800), "recorded ban takes corvus-control's expiry")

-- an acknowledgement for an older record does not drop a newer one
bans:set("192.0.2.99", "bb", 900)
q:set("p:192.0.2.99", "rec99", 900)
JSON["rec99"] = { id = "new" }
push({ ips = {}, removed = {}, ack = { ["192.0.2.99"] = "old" } })
eq(q:get("p:192.0.2.99"), "rec99", "newer pending record kept")
eq(bans:get("192.0.2.99"), "bb", "its ban kept")

-- a full push after all that leaves exactly the list (and pending local bans)
push({ ips = { ["203.0.113.5"] = 0 } })
eq(bans.sweeps, 2, "full push sweeps")
local keys = {}
for _, k in ipairs(bans:get_keys(0)) do keys[#keys + 1] = k end
table.sort(keys)
eq(table.concat(keys, " "), "192.0.2.99 203.0.113.5", "store after full push")

-- errors
eq(push({}), "bad request", "missing ips")
eq(ngx.status, 400, "bad request status")
ngx.shared = { corvus_bb_q = q }
eq(push({ ips = {} }), "no ban store", "missing shared dict")
eq(ngx.status, 503, "no ban store status")
ngx.shared = { corvus_bans = bans, corvus_bb_q = q }

-- pending bans with the shared-memory epoch
out = nil
rules.bad_behaviour_hits()
local epoch = out:match('"epoch":"([^"]+)"')
eq(out:sub(1, 15), '{"hits":[rec99]', "pending records listed")
eq(epoch ~= nil, true, "epoch reported")
T.set_time(now + 100)
rules.bad_behaviour_hits()
eq(out:match('"epoch":"([^"]+)"'), epoch, "epoch stable")
ngx.shared = { corvus_bans = dict(), corvus_bb_q = dict() } -- restarted gateway
rules.bad_behaviour_hits()
eq(out, '{"hits":[],"epoch":"' .. out:match('"epoch":"([^"]+)"') .. '"}', "empty list")
eq(out:match('"epoch":"([^"]+)"') ~= epoch, true, "new shared memory, new epoch")

-- Internal endpoints require the per-start key, so they are not reachable through a
-- proxied target pointing at the status port.
local function denied(what, header, key)
  ngx.var.http_x_corvus_status_key, STATUS_KEY = header, key
  local before = bans.sets
  ngx.shared = { corvus_bans = bans, corvus_bb_q = q, corvus_ua = dict() }
  for name, call in pairs({
    bans = function() return push({ ips = { ["198.51.100.1"] = 0 } }) end,
    hits = function() out, ngx.status = nil, 200; rules.bad_behaviour_hits(); return out end,
    ua = function() out, ngx.status = nil, 200; body = "ua"; JSON.ua = { hosts = {} }; rules.set_under_attack(); return out end,
  }) do
    eq(call(), "forbidden", what .. ": " .. name)
    eq(ngx.status, 403, what .. ": " .. name .. " status")
  end
  eq(bans.sets, before, what .. ": nothing changed")
end
denied("no header", nil, "k3y-0123456789abcdef")
denied("wrong key", "k3y-0123456789abcdeX", "k3y-0123456789abcdef")
denied("shorter key", "k3y", "k3y-0123456789abcdef")
denied("empty key in env, empty header", "", "")
denied("no key in env", "anything", nil)
ngx.var.http_x_corvus_status_key, STATUS_KEY = "k3y-0123456789abcdef", "k3y-0123456789abcdef"
eq(push({ ips = {} }), "0", "right key accepted")
eq(ngx.status, 200, "right key status")

T.done("bans_test")
