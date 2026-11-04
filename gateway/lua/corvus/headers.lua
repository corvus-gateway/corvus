-- Corvus security header presets and cookie flags (per proxy host, header_filter phase).
--
-- Levels (proxy host field security_headers):
--   off    → nothing is changed
--   basic  → X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy;
--            the upstream's X-Powered-By is removed
--   strict → basic + Content-Security-Policy-Report-Only, Cross-Origin-Opener-Policy, and
--            HttpOnly / SameSite=Lax / Secure (HTTPS only) added to upstream Set-Cookie
--            values that do not already carry the attribute
--
-- Approach: every header is set from Lua and ONLY when the response does not already
-- have it, so an application's own policy (e.g. a real CSP or X-Frame-Options DENY)
-- always wins. header_filter runs for every response, including gateway-generated
-- error pages, so the headers are present there too (like `add_header ... always`).
-- Note: headers added by nginx `add_header` (HSTS, advanced configuration) are applied
-- after this filter and are not seen here.
--
-- cookie_flags() is pure (no ngx.*) so it can be tested with plain LuaJIT
-- (gateway/test/headers_test.lua).

local find, lower, match, gsub, gmatch = string.find, string.lower, string.match, string.gsub, string.gmatch
local concat = table.concat

local _M = {}

local BASIC = {
  { "X-Content-Type-Options", "nosniff" },
  { "X-Frame-Options", "SAMEORIGIN" },
  { "Referrer-Policy", "strict-origin-when-cross-origin" },
  -- Opt out of interest-based ad tracking (Topics API, successor of FLoC's interest-cohort).
  { "Permissions-Policy", "browsing-topics=()" },
}

local STRICT = {
  -- Report-only: violations are reported in the browser console, nothing is blocked.
  { "Content-Security-Policy-Report-Only",
    "default-src 'self'; object-src 'none'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'" },
  { "Cross-Origin-Opener-Policy", "same-origin" },
}

-- Adds HttpOnly, SameSite=Lax and (when secure) Secure to one Set-Cookie value, each
-- only if the cookie does not set that attribute already (attribute names are matched
-- case-insensitively; the leading name=value pair is never treated as an attribute).
function _M.cookie_flags(cookie, secure)
  if type(cookie) ~= "string" or not find(cookie, "=", 1, true) then return cookie end
  local has, first = {}, true
  for part in gmatch(cookie, "[^;]+") do
    if first then
      first = false
    else
      local name = match(part, "^%s*([^=%s]+)")
      if name then has[lower(name)] = true end
    end
  end
  local add = {}
  if not has.httponly then add[#add + 1] = "HttpOnly" end
  if not has.samesite then add[#add + 1] = "SameSite=Lax" end
  if secure and not has.secure then add[#add + 1] = "Secure" end
  if #add == 0 then return cookie end
  local base = gsub(cookie, "[%s;]+$", "")
  return base .. "; " .. concat(add, "; ")
end

local function set_missing(h, list)
  for i = 1, #list do
    local name = list[i][1]
    if h[name] == nil then h[name] = list[i][2] end
  end
end

-- header_filter_by_lua entry point; level is rendered into the host config.
function _M.header_filter(level)
  if level ~= "basic" and level ~= "strict" then return end
  local h = ngx.header
  h["X-Powered-By"] = nil
  set_missing(h, BASIC)
  if level ~= "strict" then return end
  set_missing(h, STRICT)
  local cookies = h["Set-Cookie"]
  if cookies == nil then return end
  local secure = ngx.var.https == "on"
  if type(cookies) == "table" then
    local out = {}
    for i = 1, #cookies do out[i] = _M.cookie_flags(cookies[i], secure) end
    h["Set-Cookie"] = out
  else
    h["Set-Cookie"] = _M.cookie_flags(cookies, secure)
  end
end

return _M
