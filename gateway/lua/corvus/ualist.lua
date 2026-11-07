-- User-Agent lists for security rules ("User-Agent in list"): plain, case-insensitive
-- substring matching. No patterns or regular expressions (no ReDoS); each entry is
-- tested with string.find(..., plain = true) and the scan stops at the first hit.
-- Lists are capped by corvus-control (10,000 entries of at most 256 bytes), which keeps
-- the linear scan cheap for the few thousand entries typical bad-bot lists have.
-- Pure Lua (no ngx): unit-tested by gateway/test/ualist_test.lua.
local find, lower, type = string.find, string.lower, type

local _M = {}

-- new builds a matcher from entries (already lower-cased by the compiler; lower-cased
-- again here so hand-written rules.json behave the same).
function _M.new(entries)
  local m = {}
  if type(entries) ~= "table" then return m end
  for _, e in ipairs(entries) do
    if type(e) == "string" and e ~= "" then m[#m + 1] = lower(e) end
  end
  return m
end

-- match reports whether the lower-cased User-Agent ua_lower contains any entry.
function _M.match(m, ua_lower)
  if not ua_lower or ua_lower == "" then return false end
  for i = 1, #m do
    if find(ua_lower, m[i], 1, true) then return true end
  end
  return false
end

return _M
