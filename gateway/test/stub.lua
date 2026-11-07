-- Test helper (not a test itself): stand-ins for the OpenResty pieces the pure gateway
-- helpers use, so they can be unit-tested with plain LuaJIT.
--   resty.sha256 → pure-Lua SHA-256 (same new/update/final interface)
--   resty.random → deterministic bytes
--   ngx          → encode_base64, time (settable), log, var
local bit = require "bit"
local band, bor, bxor, bnot, rshift, lshift, ror = bit.band, bit.bor, bit.bxor, bit.bnot, bit.rshift, bit.lshift, bit.ror
local byte, char, rep = string.byte, string.char, string.rep

local K = {
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

local function sha256(msg)
  local len = #msg
  local bits = len * 8
  msg = msg .. "\128" .. rep("\0", (55 - len) % 64)
  -- 64-bit big-endian length (messages here are far below 2^32 bits)
  msg = msg .. "\0\0\0\0" .. char(band(rshift(bits, 24), 255), band(rshift(bits, 16), 255), band(rshift(bits, 8), 255), band(bits, 255))
  local H = { 0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19 }
  local W = {}
  for blk = 1, #msg, 64 do
    for i = 0, 15 do
      local a, b, c, d = byte(msg, blk + i * 4, blk + i * 4 + 3)
      W[i] = bor(lshift(a, 24), lshift(b, 16), lshift(c, 8), d)
    end
    for i = 16, 63 do
      local x, y = W[i - 15], W[i - 2]
      local s0 = bxor(ror(x, 7), ror(x, 18), rshift(x, 3))
      local s1 = bxor(ror(y, 17), ror(y, 19), rshift(y, 10))
      W[i] = bit.tobit(W[i - 16] + s0 + W[i - 7] + s1)
    end
    local a, b, c, d, e, f, g, h = H[1], H[2], H[3], H[4], H[5], H[6], H[7], H[8]
    for i = 0, 63 do
      local S1 = bxor(ror(e, 6), ror(e, 11), ror(e, 25))
      local ch = bxor(band(e, f), band(bnot(e), g))
      local t1 = bit.tobit(h + S1 + ch + K[i + 1] + W[i])
      local S0 = bxor(ror(a, 2), ror(a, 13), ror(a, 22))
      local mj = bxor(band(a, b), band(a, c), band(b, c))
      local t2 = bit.tobit(S0 + mj)
      h, g, f, e, d, c, b, a = g, f, e, bit.tobit(d + t1), c, b, a, bit.tobit(t1 + t2)
    end
    H[1], H[2], H[3], H[4] = bit.tobit(H[1] + a), bit.tobit(H[2] + b), bit.tobit(H[3] + c), bit.tobit(H[4] + d)
    H[5], H[6], H[7], H[8] = bit.tobit(H[5] + e), bit.tobit(H[6] + f), bit.tobit(H[7] + g), bit.tobit(H[8] + h)
  end
  local out = {}
  for i = 1, 8 do
    local v = H[i]
    out[i] = char(band(rshift(v, 24), 255), band(rshift(v, 16), 255), band(rshift(v, 8), 255), band(v, 255))
  end
  return table.concat(out)
end

package.preload["resty.sha256"] = function()
  local M = {}
  M.__index = M
  function M.new() return setmetatable({ buf = {} }, M) end
  function M:update(s) self.buf[#self.buf + 1] = s; return true end
  function M:final() return sha256(table.concat(self.buf)) end
  return M
end

package.preload["resty.random"] = function()
  local n = 0
  return { bytes = function(len)
    n = n + 1
    local t = {}
    for i = 1, len do t[i] = char((i * 37 + n * 11) % 256) end
    return table.concat(t)
  end }
end

local B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
local function encode_base64(s)
  local out = {}
  for i = 1, #s, 3 do
    local a, b, c = byte(s, i, i + 2)
    local n = lshift(a, 16) + lshift(b or 0, 8) + (c or 0)
    local q = { rshift(n, 18), band(rshift(n, 12), 63), band(rshift(n, 6), 63), band(n, 63) }
    out[#out + 1] = B64:sub(q[1] + 1, q[1] + 1) .. B64:sub(q[2] + 1, q[2] + 1)
      .. (b and B64:sub(q[3] + 1, q[3] + 1) or "=") .. (c and B64:sub(q[4] + 1, q[4] + 1) or "=")
  end
  return table.concat(out)
end

local now = 1700000000
ngx = {
  WARN = 5, ERR = 4,
  encode_base64 = encode_base64,
  time = function() return now end,
  log = function() end,
  var = {},
  ctx = {},
}

local M = { sha256 = sha256 }
function M.set_time(t) now = t end

M.failed = 0
function M.eq(got, want, what)
  if got ~= want then
    M.failed = M.failed + 1
    io.stderr:write(("FAIL %s\n  got:  %s\n  want: %s\n"):format(what, tostring(got), tostring(want)))
  end
end
function M.done(name)
  if M.failed > 0 then
    io.stderr:write(M.failed .. " test(s) failed\n")
    os.exit(1)
  end
  print(name .. ": ok")
end
return M
