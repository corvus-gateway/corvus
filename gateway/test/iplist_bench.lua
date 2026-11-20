-- Micro-benchmark for IP list lookups (not run by CI). From the repository root, inside
-- the gateway image (OpenSSL's libcrypto gives a real SHA-256 for the file check):
--   docker run --rm --entrypoint sh -v "$PWD:/src" -w /src ghcr.io/corvus-gateway/gateway:<v> \
--     -c '/usr/local/openresty/luajit/bin/luajit gateway/test/iplist_bench.lua [ranges]'
-- Compares, through the same ip.src condition code (corvus/rules.lua):
--   - a range file of 4 million ranges (corvus/iplist.lua, binary search, mmap)
--   - a small range file (50 ranges)
--   - networks written in the rule (Lua tables per prefix length, as before range files)
-- and reports the Lua heap that 1 million networks cost in the table representation.
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local ffi = require "ffi"
local T = require("stub")

-- real SHA-256 when libcrypto is available (the stub's pure-Lua one is slow on 32 MB)
local okc, crypto = pcall(ffi.load, "libcrypto.so.3")
if okc then
  ffi.cdef [[
    typedef struct { unsigned int h[8]; unsigned int Nl, Nh; unsigned int data[16]; unsigned int num, md_len; } BENCH_SHA256_CTX;
    int SHA256_Init(BENCH_SHA256_CTX *c);
    int SHA256_Update(BENCH_SHA256_CTX *c, const void *data, size_t len);
    int SHA256_Final(unsigned char *md, BENCH_SHA256_CTX *c);
  ]]
  package.loaded["resty.sha256"] = {
    new = function()
      local ctx = ffi.new("BENCH_SHA256_CTX")
      crypto.SHA256_Init(ctx)
      return {
        update = function(_, s) return crypto.SHA256_Update(ctx, s, #s) == 1 end,
        final = function()
          local md = ffi.new("unsigned char[32]")
          crypto.SHA256_Final(md, ctx)
          return ffi.string(md, 32)
        end,
      }
    end,
  }
end
local sha = function(s)
  local h = require("resty.sha256").new()
  h:update(s)
  return h:final()
end
local function tohex(s) return (s:gsub(".", function(c) return string.format("%02x", c:byte()) end)) end

for _, m in ipairs({ "cjson.safe", "corvus.body", "corvus.ualist", "corvus.challenge",
  "corvus.underattack", "corvus.accesslog", "corvus.ipkey" }) do
  package.loaded[m] = {}
end
ngx.re = { find = function() return nil end }
local iplist = require("corvus.iplist")
local rules = require("corvus.rules")
local compile, test = rules._compile_cond, rules._test

local N = tonumber(arg[1]) or 4000000
math.randomseed(7)

-- n sorted single-address IPv4 ranges with random gaps; returns path, hash, addresses
local function make(n, path)
  local a = ffi.new("uint32_t[?]", n * 2)
  local x = 16777216
  local step = math.floor(4000000000 / n)
  for i = 0, n - 1 do
    x = x + 1 + math.random(0, step - 2)
    a[i * 2], a[i * 2 + 1] = x, x
  end
  local payload = ffi.string(a, n * 8)
  local function u32(v) return string.char(v % 256, math.floor(v / 256) % 256, math.floor(v / 65536) % 256, math.floor(v / 16777216) % 256) end
  local hdr = "CVIPSET\0" .. u32(1) .. u32(0) .. u32(n) .. u32(0) .. u32(0) .. u32(0) .. sha(payload)
  local data = hdr .. payload
  local f = assert(io.open(path, "wb"))
  f:write(data)
  f:close()
  return path, tohex(sha(data)), a
end

local function bin4(v)
  return string.char(math.floor(v / 16777216) % 256, math.floor(v / 65536) % 256, math.floor(v / 256) % 256, v % 256)
end

-- lookup addresses: half listed, half random
local function probes(a, n, count)
  local out = {}
  for i = 1, count do
    if i % 2 == 0 then out[i] = bin4(a[math.random(0, n - 1) * 2])
    else out[i] = bin4(math.random(16777216, 4294967295)) end
  end
  return out
end

local function bench(name, c, addrs)
  local hits = 0
  local t0 = os.clock()
  for i = 1, #addrs do
    ngx.var = { binary_remote_addr = addrs[i] }
    if test({}, c, {}) then hits = hits + 1 end
  end
  local dt = os.clock() - t0
  print(string.format("%-34s %10.0f lookups/s  (%d of %d matched)", name, #addrs / dt, hits, #addrs))
end

local LOOKUPS = 2000000
local t0 = os.clock()
local big, bighash, bigarr = make(N, os.tmpname())
print(string.format("generated %d ranges (%.1f MB) in %.1f s", N, (64 + N * 8) / 1e6, os.clock() - t0))
collectgarbage()
local heap0 = collectgarbage("count")
t0 = os.clock()
local l = assert(iplist.open(big, bighash))
print(string.format("open + SHA-256 + order check: %.0f ms (%s)", (os.clock() - t0) * 1000, okc and "OpenSSL" or "pure-Lua SHA-256"))
collectgarbage()
print(string.format("Lua heap for the mapped %d-range list: %+.2f MB (the file is shared page cache)", N, (collectgarbage("count") - heap0) / 1024))

local small, smallhash, smallarr = make(50, os.tmpname())
local cbig = compile({ f = "ip.src", op = "in", v = {}, l = { { p = big, h = bighash } } })
local csmall = compile({ f = "ip.src", op = "in", v = {}, l = { { p = small, h = smallhash } } })
local nets = {}
for i = 0, 49 do nets[#nets + 1] = string.format("%08x/32", smallarr[i * 2]) end
local cinline = compile({ f = "ip.src", op = "in", v = nets })

bench(string.format("range file, %d ranges", N), cbig, probes(bigarr, N, LOOKUPS))
bench("range file, 50 ranges", csmall, probes(smallarr, 50, LOOKUPS))
bench("rule networks (tables), 50", cinline, probes(smallarr, 50, LOOKUPS))
-- the raw lookup without the condition code
local addrs = probes(bigarr, N, LOOKUPS)
t0 = os.clock()
local hits = 0
for i = 1, #addrs do if iplist.has(l, addrs[i]) then hits = hits + 1 end end
print(string.format("%-34s %10.0f lookups/s", "iplist.has only, " .. N .. " ranges", #addrs / (os.clock() - t0)))

-- the table representation of 1 million networks (what every worker built before)
local M = math.min(N, 1000000)
nets = {}
for i = 0, M - 1 do nets[i + 1] = string.format("%08x/32", bigarr[i * 2]) end
collectgarbage()
local before = collectgarbage("count")
t0 = os.clock()
local ctab = compile({ f = "ip.src", op = "in", v = nets })
local built = os.clock() - t0
collectgarbage()
print(string.format("tables for %d networks: +%.1f MB Lua heap, built in %.1f s (per worker)", M, (collectgarbage("count") - before) / 1024, built))
bench(string.format("rule networks (tables), %d", M), ctab, probes(bigarr, M, LOOKUPS))
os.remove(big)
os.remove(small)
T.done("iplist_bench")
