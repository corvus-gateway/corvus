-- Corvus — client keys derived from the client address.
-- An IPv6 host can use any address of its /64 (privacy addresses), so per-client limits
-- and automatic bans key IPv6 clients by their /64 and IPv4 clients by their address.

local _M = {}

-- First n groups of an IPv6 address, each in full four-digit lower-case form and joined
-- by ":" ("2001:db8::1", 4 → "2001:0db8:0000:0000"). nil if ip is not a plain IPv6
-- address (embedded IPv4 forms are not handled).
function _M.v6_prefix(ip, n)
  local head, tail = ip:match("^([%x:]*)::([%x:]*)$")
  local groups = {}
  local function add(part)
    if part == "" then return true end
    for g in (part .. ":"):gmatch("([^:]*):") do
      if not g:match("^%x%x?%x?%x?$") then return false end
      groups[#groups + 1] = ("0000" .. g):sub(-4):lower()
    end
    return true
  end
  if head then
    local before = {}
    if not add(head) then return nil end
    before, groups = groups, {}
    if not add(tail) then return nil end
    local after = groups
    if #before + #after > 7 then return nil end
    groups = before
    for _ = 1, 8 - #before - #after do groups[#groups + 1] = "0000" end
    for _, g in ipairs(after) do groups[#groups + 1] = g end
  elseif not add(ip) or #groups ~= 8 then
    return nil
  end
  return table.concat(groups, ":", 1, n)
end

-- Client key: the IPv4 address, or the IPv6 /64 ("2001:0db8:0001:0002").
function _M.client(ip)
  ip = ip or ""
  if ip:find(":", 1, true) then return _M.v6_prefix(ip, 4) or ip end
  return ip
end

return _M
