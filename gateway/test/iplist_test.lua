-- Unit tests for corvus/iplist.lua (IP list range files mapped with FFI) and its use by
-- ip.src conditions in corvus/rules.lua. Run with plain LuaJIT from the repository root
-- (CI lua job):
--   luajit gateway/test/iplist_test.lua
-- testdata/iplist.bin is written by the Go tests (control/internal/ipset, TestGoldenFile)
-- from the networks listed in GOLDEN below: both sides must agree on the format.
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local eq = T.eq
local iplist = require("corvus.iplist")

local GOLDEN = "gateway/test/testdata/iplist.bin"
-- 0.0.0.0, 10.0.0.0/8, 11.0.0.0/8, 192.0.2.1, 192.0.2.3, 198.51.100.0/24, 255.255.255.255,
-- ::, 2001:db8::/48, 2001:db8:0:0:ffff::/80, 2001:db8:1::5, ffff:…:ffff
local GOLDEN_SHA = "648182455c7c6f448cef8cff038c87abec0f082058bbb2d612551360677d18d9"

local function read(p)
  local f = assert(io.open(p, "rb"))
  local d = f:read("*a")
  f:close()
  return d
end
local function write(p, d)
  local f = assert(io.open(p, "wb"))
  f:write(d)
  f:close()
end
local function hex(s) return (s:gsub("..", function(x) return string.char(tonumber(x, 16)) end)) end
local function tohex(s) return (s:gsub(".", function(c) return string.format("%02x", c:byte()) end)) end
local function v4(s)
  local a, b, c, d = s:match("^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
  return string.char(tonumber(a), tonumber(b), tonumber(c), tonumber(d))
end
local function u32(n) return string.char(n % 256, math.floor(n / 256) % 256, math.floor(n / 65536) % 256, math.floor(n / 16777216) % 256) end
local function u64(n) return u32(n % 4294967296) .. u32(math.floor(n / 4294967296)) end

-- the golden file: lookups at every boundary
local data = read(GOLDEN)
eq(tohex(T.sha256(data)), GOLDEN_SHA, "golden file unchanged")
local l, err = iplist.open(GOLDEN, GOLDEN_SHA)
eq(err, nil, "golden file opens")
eq(l and l.n4, 6, "IPv4 ranges (adjacent /8s merged)")
eq(l and l.n6, 4, "IPv6 ranges (nested /80 merged)")
for addr, want in pairs({
  ["0.0.0.0"] = true, ["0.0.0.1"] = false, ["9.255.255.255"] = false, ["10.0.0.0"] = true,
  ["10.200.3.4"] = true, ["11.255.255.255"] = true, ["12.0.0.0"] = false,
  ["192.0.2.0"] = false, ["192.0.2.1"] = true, ["192.0.2.2"] = false, ["192.0.2.3"] = true,
  ["198.51.99.255"] = false, ["198.51.100.0"] = true, ["198.51.100.255"] = true, ["198.51.101.0"] = false,
  ["255.255.255.254"] = false, ["255.255.255.255"] = true,
}) do
  eq(iplist.has(l, v4(addr)), want, "IPv4 " .. addr)
end
for addr, want in pairs({
  ["00000000000000000000000000000000"] = true,   -- ::
  ["00000000000000000000000000000001"] = false,  -- ::1
  ["20010db7ffffffffffffffffffffffff"] = false,
  ["20010db8000000000000000000000000"] = true,   -- 2001:db8:: (first)
  ["20010db800000000ffff000000000001"] = true,   -- inside the nested /80
  ["20010db80000ffffffffffffffffffff"] = true,   -- last of the /48
  ["20010db8000100000000000000000000"] = false,
  ["20010db8000100000000000000000005"] = true,
  ["20010db8000100000000000000000006"] = false,
  ["fffffffffffffffffffffffffffffffe"] = false,
  ["ffffffffffffffffffffffffffffffff"] = true,
}) do
  eq(iplist.has(l, hex(addr)), want, "IPv6 " .. addr)
end
eq(iplist.has(l, "abc"), false, "malformed address")

-- an empty list matches nothing
local tmp = os.tmpname()
local function file(payload, n4, n6)
  return "CVIPSET\0" .. u32(1) .. u32(0) .. u64(n4) .. u64(n6) .. T.sha256(payload) .. payload
end
local empty = file("", 0, 0)
write(tmp, empty)
local e = iplist.open(tmp, tohex(T.sha256(empty)))
eq(e ~= nil, true, "empty list opens")
eq(e and iplist.has(e, v4("1.2.3.4")), false, "empty list: IPv4")
eq(e and iplist.has(e, hex("20010db8000000000000000000000001")), false, "empty list: IPv6")

-- a single IPv4 range, binary search on one element
local one = file(u32(16909060) .. u32(16909060), 1, 0)  -- 1.2.3.4
write(tmp, one)
local o = iplist.open(tmp, nil)
eq(o and iplist.has(o, v4("1.2.3.4")), true, "single range: hit")
eq(o and iplist.has(o, v4("1.2.3.5")), false, "single range: after")
eq(o and iplist.has(o, v4("1.2.3.3")), false, "single range: before")
eq(o and iplist.has(o, hex("00000000000000000000000000000000")), false, "single range: other family")

-- damaged files are refused with a reason
local function refused(d, want, what)
  write(tmp, d)
  local x, why = iplist.open(tmp, want)
  eq(x, nil, what)
  eq(type(why), "string", what .. ": reason")
end
refused(data, string.rep("0", 64), "checksum mismatch")
refused("X" .. data:sub(2), nil, "bad magic")
refused(data:sub(1, 8) .. u32(2) .. data:sub(13), nil, "unknown version")
refused(data:sub(1, #data - 1), nil, "truncated")
refused(data .. "\0", nil, "trailing bytes")
refused(data:sub(1, 40), nil, "shorter than a header")
-- unsorted IPv4 ranges (checksum not checked here: the order check must catch it)
refused(file(u32(10) .. u32(20) .. u32(5) .. u32(6), 2, 0), nil, "unsorted ranges")
refused(file(u32(10) .. u32(20) .. u32(20) .. u32(30), 2, 0), nil, "overlapping ranges")
refused(file(u32(30) .. u32(20), 1, 0), nil, "first > last")
local x, why = iplist.open("/nonexistent/iplist.bin", nil)
eq(x, nil, "missing file")
eq(why and why:find("cannot open", 1, true) ~= nil, true, "missing file: reason")
os.remove(tmp)

-- ip.src conditions in corvus/rules.lua: inline networks and list files together
for _, m in ipairs({ "cjson.safe", "corvus.body", "corvus.ualist", "corvus.challenge",
  "corvus.underattack", "corvus.accesslog", "corvus.ipkey" }) do
  package.loaded[m] = {}
end
ngx.re = { find = function() return nil end }
local rules = require("corvus.rules")
local compile, test = rules._compile_cond, rules._test
local ref = { { p = GOLDEN, h = GOLDEN_SHA } }
local cin = compile({ f = "ip.src", op = "in", v = { "cb007100/24" }, l = ref })  -- 203.0.113.0/24 inline
local cout = compile({ f = "ip.src", op = "not_in", v = {}, l = ref })
local function run(c, bin)
  ngx.var = { binary_remote_addr = bin }
  return test({}, c, {})
end
eq(run(cin, v4("203.0.113.9")), true, "rule: inline network")
eq(run(cin, v4("10.1.2.3")), true, "rule: list file")
eq(run(cin, v4("12.0.0.1")), false, "rule: neither")
eq(run(cout, v4("10.1.2.3")), false, "rule not_in: listed")
eq(run(cout, v4("12.0.0.1")), true, "rule not_in: not listed")
eq(run(cin, hex("20010db8000100000000000000000005")), true, "rule: IPv6 list entry")
local ok, cerr = pcall(compile, { f = "ip.src", op = "in", v = {}, l = { { p = GOLDEN, h = string.rep("1", 64) } } })
eq(ok, false, "a damaged list file rejects the configuration")
eq(tostring(cerr):find("IP list file", 1, true) ~= nil, true, "rejection names the file")

T.done("iplist_test")
