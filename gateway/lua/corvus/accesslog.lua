-- Access log of proxy hosts, batched per worker (log_by_lua, see corvus/rules.lua _M.log).
--
-- nginx's syslog output sends one datagram per request; at high request rates the
-- datagram, the receiver's wake-up and its read cost more CPU than proxying the request.
-- Here each worker collects the records in memory and a timer sends them to corvus-control
-- in datagrams of up to CHUNK bytes (newline-separated records), so one datagram carries
-- about a hundred requests.
--
-- Record format (same as log_format corvus_log in nginx.conf; parsed by
-- control/internal/traffic/parse.go — keep the field order):
--   c1 \t msec \t host \t method \t uri \t status \t body_bytes \t request_time \t ip \t wr \t
--   exploit \t limit_req \t rule \t country \t body \t ban \t user_agent
-- Values are escaped like nginx's escape=json, so they never contain a raw tab or newline.
--
-- Lossy by design, like the syslog output: when corvus-control is not listening or cannot
-- keep up (the send would block), records are dropped; requests are never delayed.

local find, gsub, sub, char, format = string.find, string.gsub, string.sub, string.char, string.format
local concat = table.concat
local tostring = tostring

local _M = {}

local CHUNK = 16384      -- bytes per datagram (a full buffer is sent at once)
local INTERVAL = 0.5     -- seconds: flush of a partly filled buffer
local MAX_URI, MAX_UA = 1024, 512  -- corvus-control keeps less (traffic.go)

local PATH            -- unix datagram socket of corvus-control
local buf, nbuf, size = {}, 0, 0
local started = false
local warned = 0

-- nginx escape=json: " \ and control characters
local ESC = { ['"'] = '\\"', ["\\"] = "\\\\", ["\n"] = "\\n", ["\r"] = "\\r", ["\t"] = "\\t", ["\b"] = "\\b", ["\f"] = "\\f" }
for i = 0, 31 do
  local c = char(i)
  if not ESC[c] then ESC[c] = format("\\u%04x", i) end
end
local NEED = '[%z\1-\31"\\]'

local function esc(s, max)
  if s == nil then return "" end
  if #s > max then s = sub(s, 1, max) end
  if find(s, NEED) then s = gsub(s, NEED, ESC) end
  return s
end
_M.escape = esc  -- unit tests

local function send(premature, lines)
  local sock = ngx.socket.udp()
  local ok, err = sock:setpeername("unix:" .. PATH)
  if ok then
    ok, err = sock:send(lines)
  end
  sock:close()
  if not ok then
    local now = ngx.now()
    if now - warned > 60 then
      warned = now
      ngx.log(ngx.NOTICE, "corvus access log: records dropped: ", err)
    end
  end
end

-- Hands the buffered records to a timer (cosockets are not available in the log phase).
-- When the timer cannot be created (too many pending), the batch is dropped.
local function flush()
  if nbuf == 0 then return end
  local data = concat(buf, "", 1, nbuf)
  buf, nbuf, size = {}, 0, 0
  ngx.timer.at(0, send, data)
end

-- Periodic flush; also runs (premature) when the worker exits, e.g. on a reload.
local function tick(premature)
  if premature then
    if nbuf > 0 then send(true, concat(buf, "", 1, nbuf)) end
    nbuf = 0
    return
  end
  flush()
end

function _M.init(path)
  PATH = path
end

-- Appends the current request (log phase). wr: access queue decision ("-" if none).
function _M.record(wr)
  if not PATH then return end
  if not started then
    started = true
    ngx.timer.every(INTERVAL, tick)
  end
  local var = ngx.var
  local line = "c1\t" .. tostring(ngx.now()) .. "\t" .. (var.corvus_host or "") .. "\t" .. ngx.req.get_method()
    .. "\t" .. esc(var.uri, MAX_URI) .. "\t" .. (var.status or "0") .. "\t" .. (var.body_bytes_sent or "0")
    .. "\t" .. (var.request_time or "0") .. "\t" .. (var.remote_addr or "") .. "\t" .. wr
    .. "\t" .. (var.corvus_exploit or "") .. "\t" .. (var.limit_req_status or "") .. "\t" .. (var.corvus_rule or "")
    .. "\t" .. (var.corvus_cc or "") .. "\t" .. (var.corvus_body or "") .. "\t" .. (var.corvus_ban or "")
    .. "\t" .. esc(var.http_user_agent, MAX_UA) .. "\n"
  nbuf = nbuf + 1
  buf[nbuf] = line
  size = size + #line
  if size >= CHUNK then flush() end
end

return _M
