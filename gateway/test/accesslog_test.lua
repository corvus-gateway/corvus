-- Unit tests for corvus/accesslog.lua. Run with plain LuaJIT from the repository root
-- (CI lua job):  luajit gateway/test/accesslog_test.lua
package.path = "gateway/lua/?.lua;" .. package.path

local sent, timers, every, every_fails, logged = {}, 0, 0, 0, 0
_G.ngx = {
  now = function() return 1762243516.123 end,
  var = {},
  timer = {
    at = function(_, fn, data) timers = timers + 1; sent[#sent + 1] = data; return true end,
    every = function()
      every = every + 1
      if every_fails > 0 then
        every_fails = every_fails - 1
        return nil, "too many pending timers"
      end
      return true
    end,
  },
  log = function() logged = logged + 1 end, NOTICE = 6, ERR = 4,
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
  .. "sqli\tREJECTED_DRY_RUN\t7:block\tKR\tsig:log\t1\t/a\\\"b\\\\c\\td\tMozilla/5.0 \\u0001 ünïcode\t\t\t\t\n"
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
eq(sent[1]:match("\n(c1[^\n]*)\n$"), "c1\t1762243516.123\th\tGET\t200\t0\t0\t1.1.1.1\t-\t\t\t\t\t\t\t/\t\t\t\t\t", "absent User-Agent")
al.record()
eq(every, 1, "timer not started again")

-- traffic sources: the referrer origin from nginx ($corvus_ref) and only the three UTM
-- values, read only when the query mentions "utm_"; never the rest of the query
local function last_record()
  sent, timers = {}, 0
  while timers == 0 do al.record() end
  return sent[1]:match("\n(c1[^\n]*)\n$")
end
local base = "c1\t1\th\tGET\t200\t0\t0\t1.1.1.1\t-\t\t\t\t\t\t\t/p\tUA\t"
ngx.var = { corvus_log_fields = "1\th\tGET\t200\t0\t0\t1.1.1.1\t-\t\t\t\t\t\t", uri = "/p", http_user_agent = "UA",
  corvus_ref = "https://www.google.com", args = "utm_source=ig&token=abc&utm_campaign=fall%22x",
  arg_utm_source = "ig", arg_utm_campaign = 'fall"x\t' }
eq(last_record(), base .. 'https://www.google.com\tig\t\tfall\\"x\\t', "referrer origin and UTM values")
ngx.var.args = "q=1"
eq(last_record(), base .. "https://www.google.com\t\t\t", "UTM values not read without utm_ in the query")
-- nginx matches argument names case-insensitively: $arg_utm_source is set for UTM_Source
ngx.var.args, ngx.var.arg_utm_campaign = "UTM_Source=ig", nil
eq(last_record(), base .. "https://www.google.com\tig\t\t", "UTM values read when the query names them in upper case")
ngx.var.args = "Utm_Source=ig"
eq(last_record(), base .. "https://www.google.com\tig\t\t", "UTM values read when the query names them in mixed case")
ngx.var.args, ngx.var.arg_utm_source = "utm_source=x", string.rep("s", 400)
eq(#last_record():match("\t(s+)\t\t"), 300, "UTM value bounded")

-- the flush timer cannot be created (fresh worker): logged, and retried with the next
-- request instead of never flushing
package.loaded["corvus.accesslog"] = nil
local al2 = require("corvus.accesslog")
al2.init("/data/run/corvus-access.sock")
every, every_fails, logged = 0, 1, 0
al2.record()
eq(every, 1, "timer creation attempted")
eq(logged, 1, "failure logged")
al2.record()
eq(every, 2, "timer creation retried with the next request")
al2.record()
eq(every, 2, "timer not started again once it works")

if failed > 0 then
  io.stderr:write(failed .. " test(s) failed\n")
  os.exit(1)
end
print("accesslog_test: ok")
