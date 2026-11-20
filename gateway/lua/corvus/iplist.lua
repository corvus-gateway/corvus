-- IP lists as compact range files (written by corvus-control, control/internal/ipset):
-- sorted, disjoint, inclusive address ranges, mapped read-only into memory.
--
-- Files are opened by corvus/rules.lua at configuration load, i.e. in init_by_lua: in the
-- nginx master (and in `openresty -t`). The mapping is MAP_SHARED and PROT_READ, so every
-- worker forked from the master shares the same pages through the page cache: a list of
-- millions of entries costs its file size once, not a Lua table per worker. A lookup is
-- a binary search over the ranges of the address family (no allocation).
--
-- Lifetime: each configuration load maps the files it refers to (once per file hash).
-- On a reload the master loads the new configuration in a new Lua VM and the old one is
-- closed: the finalizer of each mapping unmaps it there. Workers of the old
-- configuration keep their inherited mappings until they exit (the kernel unmaps them
-- with the process), so a request in flight never sees a list go away.
--
-- File format (little-endian; see control/internal/ipset/ipset.go):
--   0  "CVIPSET\0"   8  version (u32 = 1)   12  reserved (u32)
--   16 n4 (u64)      24 n6 (u64)           32  SHA-256 of the payload (32 bytes)
--   64 n4 × { u32 first, u32 last }, then n6 × { u64 first_hi, first_lo, last_hi, last_lo }
-- An IPv6 address is (hi, lo): bytes 0-7 and 8-15 read as big-endian numbers.
--
-- open() refuses (returns nil, err) a file that is missing, of the wrong size, damaged
-- (SHA-256 of the whole file different from the name the configuration gives), or whose
-- ranges are not sorted and disjoint. corvus/rules.lua then rejects the configuration.

local ffi = require "ffi"
local bit = require "bit"
local C = ffi.C
local byte, sub = string.byte, string.sub
local bswap, rshift = bit.bswap, bit.rshift
local tonumber = tonumber

for _, decl in ipairs({
  "int open(const char *path, int flags, ...);",
  "int close(int fd);",
  "int64_t lseek(int fd, int64_t offset, int whence);",
  "void *mmap(void *addr, size_t length, int prot, int flags, int fd, int64_t offset);",
  "int munmap(void *addr, size_t length);",
  "char *strerror(int errnum);",
}) do
  pcall(ffi.cdef, decl)
end

local O_RDONLY, O_CLOEXEC = 0, 0x80000   -- Linux (x86-64, arm64)
local PROT_READ, MAP_SHARED, SEEK_END = 1, 1, 2
local MAGIC, HEADER = "CVIPSET\0", 64
local u8p, u32p, u64p = ffi.typeof("const uint8_t *"), ffi.typeof("const uint32_t *"), ffi.typeof("const uint64_t *")

local _M = {}

local function errstr()
  return ffi.string(C.strerror(ffi.errno()))
end

local function tohex(s)
  return (s:gsub(".", function(c) return string.format("%02x", byte(c)) end))
end

-- SHA-256 of len bytes at p, in 1 MiB pieces (resty.sha256 takes strings).
local function sha256_hex(p, len)
  local sha = require("resty.sha256"):new()
  local off, step = 0, 1048576
  while off < len do
    local n = len - off
    if n > step then n = step end
    sha:update(ffi.string(p + off, n))
    off = off + n
  end
  return tohex(sha:final())
end

-- ranges must be sorted, first <= last, and not overlap the previous range
local function check_order(l)
  local a = l.v4
  local prev = -1
  for i = 0, l.n4 - 1 do
    local f, t = a[i * 2], a[i * 2 + 1]
    if f > t or f <= prev then return false end
    prev = t
  end
  local b = l.v6
  for i = 0, l.n6 - 1 do
    local o = i * 4
    local fh, fl, th, tl = b[o], b[o + 1], b[o + 2], b[o + 3]
    if fh > th or (fh == th and fl > tl) then return false end
    if i > 0 then
      local ph, pl = b[o - 2], b[o - 1]   -- last of the previous range
      if fh < ph or (fh == ph and fl <= pl) then return false end
    end
  end
  return true
end

-- open(path, sha256) → list | nil, err. sha256 (hex) is the expected hash of the file.
function _M.open(path, want)
  if not ffi.abi("le") then return nil, "big-endian systems are not supported" end
  local fd = C.open(path, O_RDONLY + O_CLOEXEC)
  if fd < 0 then return nil, "cannot open: " .. errstr() end
  local size = tonumber(C.lseek(fd, 0, SEEK_END))
  if size < HEADER then
    C.close(fd)
    return nil, size < 0 and ("cannot read: " .. errstr()) or "file too short"
  end
  local m = C.mmap(nil, size, PROT_READ, MAP_SHARED, fd, 0)
  local merr = (ffi.cast("intptr_t", m) == -1) and errstr() or nil
  C.close(fd)  -- the mapping stays valid
  if merr then return nil, "cannot map: " .. merr end
  -- unmapped when collected or when the Lua VM is closed (old configuration, see above)
  local base = ffi.gc(ffi.cast(u8p, m), function(p) C.munmap(ffi.cast("void *", p), size) end)
  if ffi.string(base, 8) ~= MAGIC or ffi.cast(u32p, base + 8)[0] ~= 1 then
    return nil, "not an IP list file (or an unsupported version)"
  end
  local h = ffi.cast(u64p, base + 16)
  local n4, n6 = tonumber(h[0]), tonumber(h[1])
  if n4 > 2 ^ 40 or n6 > 2 ^ 40 or HEADER + n4 * 8 + n6 * 32 ~= size then
    return nil, "file size does not match its header"
  end
  if want and sha256_hex(base, size) ~= want then
    return nil, "checksum mismatch"
  end
  local l = {
    base = base, size = size,  -- base keeps the mapping alive
    n4 = n4, n6 = n6,
    v4 = ffi.cast(u32p, base + HEADER),
    v6 = ffi.cast(u64p, base + HEADER + n4 * 8),
  }
  if not check_order(l) then
    return nil, "ranges are not sorted and disjoint"
  end
  return l
end

-- has(list, bin) → whether the binary address (4 or 16 bytes, as
-- ngx.var.binary_remote_addr) is in the list.
function _M.has(l, bin)
  local len = #bin
  if len == 4 then
    local n = l.n4
    if n == 0 then return false end
    local b1, b2, b3, b4 = byte(bin, 1, 4)
    local x = ((b1 * 256 + b2) * 256 + b3) * 256 + b4
    local a = l.v4
    if a[0] > x then return false end
    local lo, hi = 0, n - 1   -- last range whose first address is <= x
    while lo < hi do
      local mid = rshift(lo + hi + 1, 1)
      if a[mid * 2] <= x then lo = mid else hi = mid - 1 end
    end
    return x <= a[lo * 2 + 1]
  elseif len == 16 then
    local n = l.n6
    if n == 0 then return false end
    local k = ffi.cast(u64p, bin)
    local xh, xl = bswap(k[0]), bswap(k[1])
    local a = l.v6
    local fh = a[0]
    if fh > xh or (fh == xh and a[1] > xl) then return false end
    local lo, hi = 0, n - 1
    while lo < hi do
      local mid = rshift(lo + hi + 1, 1)
      local o = mid * 4
      fh = a[o]
      if fh < xh or (fh == xh and a[o + 1] <= xl) then lo = mid else hi = mid - 1 end
    end
    local o = lo * 4
    local th = a[o + 2]
    return xh < th or (xh == th and xl <= a[o + 3])
  end
  return false
end

return _M
