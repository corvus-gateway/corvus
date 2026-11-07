-- Unit tests for corvus/accesslog.lua. Run with plain LuaJIT from the repository root
-- (CI lua job):  luajit gateway/test/accesslog_test.lua
package.path = "gateway/lua/?.lua;" .. package.path

local sent, timers, every = {}, 0, 0
_G.ngx = {
  now = function() return 1762243516.123 end,
  var = {},
  timer = {
    at = function(_, fn, data) timers = timers + 1; sent[#sent + 1] = data; return true end,
    every = function() every = every + 1; return true end,
  },
  log = function() end, NOTICE = 6,
}
local al = require("corvus.accesslog")

local failed = 0
local function eq(got, want, what)
  if got ~= want then
    failed = failed + 1
    io.stderr:write(("FAIL %s\n  got:  %q\n  want: %q\n"):format(what, tostring(got), tostring(want)))
  end
end

-- nginx escape=json
eq(al.escape('a"b\\c', 100), 'a\\"b\\\\c', "quote and backslash")
eq(al.escape("x\ty\nz\r\0\1\127", 100), "x\\ty\\nz\\r\\u0000\\u0001\127", "control characters")
eq(al.escape("ünï", 100), "ünï", "UTF-8 unchanged")
eq(al.escape(nil, 10), "", "absent value")
eq(al.escape("abcdef", 3), "abc", "truncated")

-- not initialised: nothing recorded
al.record()
eq(every, 0, "no timer before init")

al.init("/data/run/corvus-access.sock")
-- $corvus_log_fields is composed by nginx (map in nginx.conf)
ngx.var = {
  corvus_log_fields = "1762243516.123\tshop.example.com\tGET\t403\t1234\t0.012\t203.0.113.9\tqueue\tsqli\t"
    .. "REJECTED_DRY_RUN\t7:block\tKR\tsig:log\t1",
  uri = '/a"b\\c\td', http_user_agent = "Mozilla/5.0 \1 ünïcode",
}
-- The same lines are parsed by control/internal/traffic/parse_test.go (TestParseLuaRecord).
local want = "c1\t1762243516.123\tshop.example.com\tGET\t403\t1234\t0.012\t203.0.113.9\tqueue\t"
  .. "sqli\tREJECTED_DRY_RUN\t7:block\tKR\tsig:log\t1\t/a\\\"b\\\\c\\td\tMozilla/5.0 \\u0001 ünïcode\n"
al.record()
eq(every, 1, "periodic flush timer started once")
eq(timers, 0, "small batch kept in memory")

-- a full buffer is handed to a timer as one datagram
ngx.var = { corvus_log_fields = "1762243516.123\th\tGET\t200\t0\t0\t1.1.1.1\t-\t\t\t\t\t\t", uri = "/" }
local n = 1
while timers == 0 and n < 1000 do al.record(); n = n + 1 end
eq(timers, 1, "flushed when the buffer is full")
eq(sent[1]:sub(1, #want), want, "record format")
local lines = 0
for _ in sent[1]:gmatch("\n") do lines = lines + 1 end
eq(lines, n, "every record in the datagram")
eq(sent[1]:match("\n(c1[^\n]*)\n$"), "c1\t1762243516.123\th\tGET\t200\t0\t0\t1.1.1.1\t-\t\t\t\t\t\t\t/\t", "absent User-Agent")
al.record()
eq(every, 1, "timer not started again")

if failed > 0 then
  io.stderr:write(failed .. " test(s) failed\n")
  os.exit(1)
end
print("accesslog_test: ok")
