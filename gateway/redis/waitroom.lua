-- Corvus (Corvus) — virtual waiting room state machine.
-- Executed atomically inside Redis (EVALSHA), so slot accounting and queue order
-- are race-free across any number of gateway workers/nodes.
--
-- KEYS (all share the {room} hash tag → same cluster slot):
--   1 active  ZSET  member=sid  score=expires_at_ms   (admitted or granted sessions)
--   2 queue   ZSET  member=sid  score=ticket          (FIFO waiting line)
--   3 seen    ZSET  member=sid  score=last_poll_ms    (abandonment detection)
--   4 seq     STRING ticket counter
--   5 rate    HASH  s=epoch_sec n=admitted_in_sec     (admission rate limit)
--   6 granted ZSET  member=sid  score=ticket          (promoted, turn not yet claimed)
-- ARGV: op, sid, capacity, active_ttl_ms, grant_ttl_ms, queue_ttl_ms, admit_rate_per_sec(0=off), promote_max, max_queue(0=off)
-- Returns: {code, rank, ticket, queue_len, active_count}
--   code: 0=NOT_FOUND 1=ACTIVE 2=WAITING 3=RELEASED 4=FULL (queue at max_queue)
-- Time comes from Redis TIME so all gateways share one clock.

local K_ACTIVE, K_QUEUE, K_SEEN, K_SEQ, K_RATE, K_GRANTED = KEYS[1], KEYS[2], KEYS[3], KEYS[4], KEYS[5], KEYS[6]
local op, sid = ARGV[1], ARGV[2]
local cap = tonumber(ARGV[3])
local active_ttl = tonumber(ARGV[4])
local grant_ttl = tonumber(ARGV[5])
local queue_ttl = tonumber(ARGV[6])
local rate = tonumber(ARGV[7])
local promote_max = tonumber(ARGV[8])
local max_queue = tonumber(ARGV[9] or "0")

local NOT_FOUND, ACTIVE, WAITING, RELEASED, FULL = 0, 1, 2, 3, 4

local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)

-- Expired sessions free their slot (TTL/heartbeat based release; browser close is never trusted).
redis.call('ZREMRANGEBYSCORE', K_ACTIVE, '-inf', now)

-- Prune a bounded number of abandoned waiters anywhere in the line (not only at
-- the head), so stale entries cannot accumulate.
local stale = redis.call('ZRANGEBYSCORE', K_SEEN, '-inf', now - queue_ttl, 'LIMIT', 0, 50)
for i = 1, #stale do
  redis.call('ZREM', K_QUEUE, stale[i])
  redis.call('ZREM', K_SEEN, stale[i])
  redis.call('ZREM', K_GRANTED, stale[i])
end

local rate_sec = math.floor(now / 1000)
local rate_used = 0
if rate > 0 then
  local r = redis.call('HMGET', K_RATE, 's', 'n')
  if tonumber(r[1]) == rate_sec then rate_used = tonumber(r[2]) or 0 end
end
local admitted_now = 0

-- How many more sessions may be admitted right now (capacity and rate limit).
local function budget()
  local free = cap - redis.call('ZCARD', K_ACTIVE)
  if rate > 0 then
    local left = rate - rate_used - admitted_now
    if left < free then free = left end
  end
  return free
end

local function admit(member, ttl)
  redis.call('ZADD', K_ACTIVE, now + ttl, member)
  admitted_now = admitted_now + 1
end

-- Move the head of the FIFO queue into free slots. Waiters that stopped polling
-- (seen older than queue_ttl) are dropped instead of being granted a slot.
-- A promoted waiter gets a short grant_ttl; it is extended to active_ttl when claimed.
-- Until then it stays in `granted` (with its ticket) and `seen`, so a turn that lapses
-- before the visitor checks in again is not lost: see restore_lapsed().
local function promote()
  local scanned = 0
  while scanned < promote_max and budget() > 0 do
    local head = redis.call('ZPOPMIN', K_QUEUE)
    if #head == 0 then break end
    scanned = scanned + 1
    local m, ticket = head[1], head[2]
    local seen = tonumber(redis.call('ZSCORE', K_SEEN, m))
    if seen and (now - seen) <= queue_ttl then
      admit(m, grant_ttl)
      redis.call('ZADD', K_GRANTED, ticket, m)
      -- the abandonment time for a lapsed turn counts from when the turn lapses
      redis.call('ZADD', K_SEEN, now + grant_ttl, m)
    else
      redis.call('ZREM', K_SEEN, m)
    end
  end
end

-- The visitor claims its slot (or is inside anyway): forget the pending turn.
local function claimed(member)
  redis.call('ZREM', K_GRANTED, member)
  redis.call('ZREM', K_SEEN, member)
end

-- A promoted visitor whose turn lapsed (slot expired before it checked in) and who is
-- back within queue_ttl goes back into the line with its original ticket, i.e. ahead
-- of everyone who came later, instead of rejoining at the back.
local function restore_lapsed(member)
  local ticket = redis.call('ZSCORE', K_GRANTED, member)
  if not ticket or redis.call('ZSCORE', K_ACTIVE, member) then return end
  redis.call('ZREM', K_GRANTED, member)
  local seen = tonumber(redis.call('ZSCORE', K_SEEN, member))
  if seen and (now - seen) <= queue_ttl then
    redis.call('ZADD', K_QUEUE, ticket, member)
    redis.call('ZADD', K_SEEN, now, member)
  else
    redis.call('ZREM', K_SEEN, member)
  end
end

local function finish(code, member)
  if rate > 0 and admitted_now > 0 then
    redis.call('HSET', K_RATE, 's', rate_sec, 'n', rate_used + admitted_now)
    redis.call('PEXPIRE', K_RATE, 5000)
  end
  local rank, ticket = 0, 0
  if code == WAITING then
    rank = redis.call('ZRANK', K_QUEUE, member) + 1
    ticket = tonumber(redis.call('ZSCORE', K_QUEUE, member))
  end
  return {code, rank, ticket, redis.call('ZCARD', K_QUEUE), redis.call('ZCARD', K_ACTIVE)}
end

if op == 'stats' then
  promote()
  return finish(NOT_FOUND, sid)
end

if op == 'release' then
  redis.call('ZREM', K_ACTIVE, sid)
  redis.call('ZREM', K_QUEUE, sid)
  redis.call('ZREM', K_SEEN, sid)
  redis.call('ZREM', K_GRANTED, sid)
  promote()
  return finish(RELEASED, sid)
end

if op == 'heartbeat' then
  -- Hot path for admitted users: extend only, no queue work.
  if redis.call('ZSCORE', K_ACTIVE, sid) then
    redis.call('ZADD', K_ACTIVE, 'XX', now + active_ttl, sid)
    return finish(ACTIVE, sid)
  end
  -- The gateway only sends a heartbeat for a valid, signed admission token that has not
  -- expired, so a missing entry means the store lost its state (restart). The visitor
  -- was inside: let them back in instead of sending them to the back of the line.
  admit(sid, active_ttl)
  claimed(sid)
  return finish(ACTIVE, sid)
end

if op ~= 'enter' and op ~= 'poll' then
  return redis.error_reply('Corvus unknown op')
end

restore_lapsed(sid)
promote()

-- Already admitted/granted (also covers multiple tabs sharing one session): claim and extend.
if redis.call('ZSCORE', K_ACTIVE, sid) then
  redis.call('ZADD', K_ACTIVE, 'XX', now + active_ttl, sid)
  claimed(sid)
  return finish(ACTIVE, sid)
end

-- Already waiting (refresh / reconnect): keep the original ticket and rank.
if redis.call('ZSCORE', K_QUEUE, sid) then
  redis.call('ZADD', K_SEEN, now, sid)
  return finish(WAITING, sid)
end

if op == 'poll' then
  return finish(NOT_FOUND, sid)
end

-- New arrival: admit directly only when nobody is waiting (no queue jumping).
if redis.call('ZCARD', K_QUEUE) == 0 and budget() > 0 then
  admit(sid, active_ttl)
  return finish(ACTIVE, sid)
end

if max_queue > 0 and redis.call('ZCARD', K_QUEUE) >= max_queue then
  return finish(FULL, sid)
end

local seq = redis.call('INCR', K_SEQ)
redis.call('ZADD', K_QUEUE, seq, sid)
redis.call('ZADD', K_SEEN, now, sid)
return finish(WAITING, sid)
