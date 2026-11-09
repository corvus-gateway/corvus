-- Unit tests for the access-queue state machine (gateway/redis/waitroom.lua), run
-- against a small in-memory stand-in for the Redis/Valkey commands it uses.
-- Run with plain LuaJIT from the repository root (CI lua job):
--   luajit gateway/test/waitroom_script_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local eq = T.eq

local chunk = assert(loadfile("gateway/redis/waitroom.lua"))

-- ---------------------------------------------------------------- fake store

local db, clock_ms = {}, 0

local function zset(k)
  db[k] = db[k] or { z = {} }
  return db[k].z
end

local function sorted(k)
  local out = {}
  for m, s in pairs(zset(k)) do out[#out + 1] = { m, s } end
  table.sort(out, function(a, b) if a[2] ~= b[2] then return a[2] < b[2] end return a[1] < b[1] end)
  return out
end

local function num(v)
  if v == "-inf" then return -math.huge end
  if v == "+inf" then return math.huge end
  return tonumber(v)
end

local cmd = {}
function cmd.TIME() return { tostring(math.floor(clock_ms / 1000)), tostring((clock_ms % 1000) * 1000) } end
function cmd.ZADD(k, ...)
  local a = { ... }
  local xx = a[1] == "XX"
  if xx then table.remove(a, 1) end
  local z = zset(k)
  for i = 1, #a, 2 do
    local s, m = tonumber(a[i]), a[i + 1]
    if not xx or z[m] then z[m] = s end
  end
  return 1
end
function cmd.ZREM(k, m) local z = zset(k); local had = z[m] ~= nil; z[m] = nil; return had and 1 or 0 end
function cmd.ZSCORE(k, m) local s = zset(k)[m]; if s == nil then return false end return tostring(s) end
function cmd.ZCARD(k) local n = 0; for _ in pairs(zset(k)) do n = n + 1 end return n end
function cmd.ZRANK(k, m)
  for i, e in ipairs(sorted(k)) do if e[1] == m then return i - 1 end end
  return false
end
function cmd.ZPOPMIN(k)
  local s = sorted(k)
  if #s == 0 then return {} end
  zset(k)[s[1][1]] = nil
  return { s[1][1], tostring(s[1][2]) }
end
function cmd.ZREMRANGEBYSCORE(k, lo, hi)
  for _, e in ipairs(sorted(k)) do if e[2] >= num(lo) and e[2] <= num(hi) then zset(k)[e[1]] = nil end end
  return 0
end
function cmd.ZRANGEBYSCORE(k, lo, hi, _, off, cnt)
  local out = {}
  for _, e in ipairs(sorted(k)) do
    if e[2] >= num(lo) and e[2] <= num(hi) then out[#out + 1] = e[1] end
  end
  local res = {}
  for i = (off or 0) + 1, math.min(#out, (off or 0) + (cnt or #out)) do res[#res + 1] = out[i] end
  return res
end
function cmd.INCR(k) db[k] = db[k] or { v = 0 }; db[k].v = db[k].v + 1; return db[k].v end
function cmd.HMGET(k, ...)
  local h, out = (db[k] or {}).h or {}, {}
  for i, f in ipairs({ ... }) do out[i] = h[f] or false end
  return out
end
function cmd.HSET(k, ...)
  db[k] = db[k] or { h = {} }
  local a = { ... }
  for i = 1, #a, 2 do db[k].h[a[i]] = tostring(a[i + 1]) end
  return 1
end
function cmd.PEXPIRE() return 1 end

local fake = { call = function(name, ...) return assert(cmd[name:upper()], name)(...) end, error_reply = function(e) return { err = e } end }

local KEYS = { "active", "queue", "seen", "seq", "rate", "granted" }
local CAP, ACTIVE_TTL, GRANT_TTL, QUEUE_TTL = 1, 120000, 30000, 120000

local function run(op, sid)
  local env = setmetatable({ KEYS = KEYS, ARGV = { op, sid, CAP, ACTIVE_TTL, GRANT_TTL, QUEUE_TTL, 0, 200, 0 }, redis = fake }, { __index = _G })
  local r = setfenv(chunk, env)()
  return { code = r[1], rank = r[2], ticket = r[3], waiting = r[4], active = r[5], new = r[6] }
end

local ACTIVE, WAITING, NOT_FOUND = 1, 2, 0
local function at(sec) clock_ms = sec * 1000 end

-- ---------------------------------------------------------------- basics
at(0)
eq(run("enter", "a").code, ACTIVE, "first visitor gets in")
eq(run("enter", "b").code, WAITING, "second visitor waits (capacity 1)")
eq(run("enter", "c").rank, 2, "third visitor is second in line")
eq(run("poll", "b").rank, 1, "poll keeps the place")

-- ---------------------------------------------------------------- claimed turn
at(5)
run("release", "a")                       -- frees the slot, b is promoted
eq(zset("granted").b ~= nil, true, "promoted visitor has a pending turn")
at(10)
eq(run("poll", "b").code, ACTIVE, "promoted visitor claims within turn validity")
eq(zset("granted").b, nil, "claimed turn is no longer pending")
eq(zset("seen").b, nil, "claimed visitor is no longer tracked as waiting")

-- ---------------------------------------------------------------- lapsed turn
at(12)
run("release", "b")                       -- c is promoted (turn until t=42)
at(13)
eq(run("enter", "d").code, WAITING, "d queues while c's turn is pending")
at(44)                                    -- c's turn lapsed; c last checked in at t=0
eq(run("enter", "e").code, WAITING, "e queues (the free slot went to d, next in line)")
local back = run("poll", "c")
eq(back.code, WAITING, "lapsed visitor is not let in over the visitor who got the slot")
eq(back.rank, 1, "lapsed visitor is back at the front, not at the back")
eq(run("poll", "e").rank, 2, "e stays behind the lapsed visitor")
eq(run("poll", "d").code, ACTIVE, "d claims its turn")

-- ---------------------------------------------------------------- next slot goes to the lapsed visitor
at(50)
run("release", "d")
eq(run("poll", "c").code, ACTIVE, "the next free slot goes to the lapsed visitor")
eq(run("poll", "e").rank, 1, "e is next")

-- ---------------------------------------------------------------- abandoned turn
at(60)
run("release", "c")                       -- e promoted, never comes back
at(60 + 200)
run("stats", "")
eq(zset("granted").e, nil, "an abandoned pending turn is cleaned up")
eq(run("poll", "e").code, NOT_FOUND, "an abandoned visitor is not restored")

-- ---------------------------------------------------------------- abandonment counts from the lapse
for k in pairs(db) do db[k] = nil end
at(1000)
run("enter", "p")                         -- p inside
eq(run("enter", "q").code, WAITING, "q waits")
eq(run("enter", "r").code, WAITING, "r waits behind q")
at(1100)                                  -- q last checked in at t=1000 (within queue_ttl)
run("poll", "r")                          -- r keeps polling
run("release", "p")                       -- q promoted at t=1100, turn lapses at t=1130
at(1131)
eq(run("poll", "r").code, ACTIVE, "the lapsed slot goes to r")
at(1200)                                  -- 200 s after q's last poll, 70 s after the lapse
local qb = run("poll", "q")
eq(qb.code .. ":" .. qb.rank, WAITING .. ":1", "q is restored at the front (window counts from the lapse)")

-- ---------------------------------------------------------------- store lost its state
for k in pairs(db) do db[k] = nil end
at(2000)
eq(run("enter", "w").code, ACTIVE, "w inside")
eq(run("enter", "x").code, WAITING, "x waits")
for k in pairs(db) do db[k] = nil end    -- Valkey restarted: everything is gone
eq(run("enter", "x").code, ACTIVE, "after the restart x comes back first and gets the slot")
eq(run("heartbeat", "w").code, ACTIVE, "w (valid admission token) is let back in, not queued")
eq(zset("active").w ~= nil, true, "w holds a slot again")

-- ---------------------------------------------------------------- new places in line
-- Only the call that puts a visitor into the line reports it as new, so statistics count
-- each place in line once, not every reload or retried request of a waiting visitor.
for k in pairs(db) do db[k] = nil end
at(3000)
eq(run("enter", "n1").new, 0, "admitted directly: not a place in line")
eq(run("enter", "n2").new, 1, "joins the line: new")
eq(run("enter", "n2").new, 0, "reload while waiting: not new")
eq(run("poll", "n2").new, 0, "status check: not new")
eq(run("poll", "n3").new, 0, "unknown visitor polling: not new")
eq(run("enter", "n3").new, 1, "next visitor joins: new")

T.done("waitroom_script_test")
