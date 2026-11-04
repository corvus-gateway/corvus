-- Corvus token primitives shared by the access queue (corvus/waitroom.lua) and the
-- security-rule challenge (corvus/challenge.lua): HMAC-SHA256 on top of resty.sha256
-- (FFI to OpenSSL, bundled with OpenResty), constant-time comparison, base64url.

local rsha256 = require "resty.sha256"
local bit = require "bit"

local byte, char, format, rep = string.byte, string.char, string.format, string.rep

local _M = {}

function _M.sha256_bin(s)
  local h = rsha256:new()
  h:update(s)
  return h:final()
end
local sha256_bin = _M.sha256_bin

function _M.to_hex(s)
  return (s:gsub(".", function(c) return format("%02x", byte(c)) end))
end

-- HMAC-SHA256 (RFC 2104); the padded key blocks are precomputed per key, so each MAC
-- costs two SHA-256 passes. kid: short public key id (first 8 hex digits of the hash
-- of the padded key) carried in tokens so a rotated-out key is still recognised.
function _M.hmac_key(secret)
  if #secret > 64 then secret = sha256_bin(secret) end
  secret = secret .. rep("\0", 64 - #secret)
  local ipad, opad = {}, {}
  for i = 1, 64 do
    local b = byte(secret, i)
    ipad[i] = char(bit.bxor(b, 0x36))
    opad[i] = char(bit.bxor(b, 0x5c))
  end
  return { ipad = table.concat(ipad), opad = table.concat(opad), kid = _M.to_hex(sha256_bin(secret)):sub(1, 8) }
end

function _M.hmac(key, msg)
  return sha256_bin(key.opad .. sha256_bin(key.ipad .. msg))
end

-- Constant-time string comparison (length is not secret: MACs have a fixed size).
function _M.const_eq(a, b)
  if type(a) ~= "string" or type(b) ~= "string" or #a ~= #b then return false end
  local r = 0
  for i = 1, #a do r = bit.bor(r, bit.bxor(byte(a, i), byte(b, i))) end
  return r == 0
end

function _M.b64url(s)
  return (ngx.encode_base64(s):gsub("[+/=]", { ["+"] = "-", ["/"] = "_", ["="] = "" }))
end

return _M
