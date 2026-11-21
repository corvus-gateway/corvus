-- Static pages of proxy hosts that serve a page instead of forwarding (content mode
-- "page"). The control plane writes the page into the configuration version directory
-- (pages/host-<id>.html); the host's location / serves it from content_by_lua, after the
-- rewrite (security rules, bans, challenge) and access (rate limit, Basic auth, access
-- queue) phases, with the configured status for every path and method:
--   content_by_lua_block { require("corvus.page").serve("/data/nginx/v12/pages/host-3.html", 503, true) }
--
-- Built-in templates contain {{HOST}}, replaced with the request's host name
-- (HTML-escaped); custom HTML is served byte for byte. Responses other than 200 are
-- never cached (Cache-Control: no-store), so a maintenance page disappears as soon as
-- the host forwards again.
--
-- Files are read once per worker and kept (a new configuration version has new paths;
-- the cache is dropped when it grows large).

local _M = {}

local CACHE_MAX = 512  -- files per worker before the cache starts over
local cache, cached = {}, 0

local HTML_ESC = { ["&"] = "&amp;", ["<"] = "&lt;", [">"] = "&gt;", ['"'] = "&quot;", ["'"] = "&#39;" }

-- load returns the file's contents (cached), or nil and an error.
function _M.load(path)
  local body = cache[path]
  if body then return body end
  local f, err = io.open(path, "rb")
  if not f then return nil, err end
  body = f:read("*a")
  f:close()
  if not body then return nil, "cannot read " .. path end
  if cached >= CACHE_MAX then cache, cached = {}, 0 end
  cache[path], cached = body, cached + 1
  return body
end

-- render fills in the host name marker of a built-in page (host HTML-escaped).
function _M.render(body, host)
  local h = string.gsub(tostring(host or ""), "[&<>\"']", HTML_ESC)
  return (string.gsub(body, "{{HOST}}", function() return h end))
end

function _M.serve(path, status, has_host)
  local body, err = _M.load(path)
  if not body then
    ngx.log(ngx.ERR, "corvus page: ", err)
    return ngx.exit(500)
  end
  if has_host then body = _M.render(body, ngx.var.host) end
  ngx.status = status
  ngx.header["Content-Type"] = "text/html; charset=utf-8"
  ngx.header["Content-Length"] = #body
  if status ~= 200 then ngx.header["Cache-Control"] = "no-store" end
  ngx.print(body)
  return ngx.exit(ngx.HTTP_OK)  -- status already set; finalize the request
end

return _M
