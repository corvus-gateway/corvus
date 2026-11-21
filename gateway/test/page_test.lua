-- Unit tests for corvus/page.lua (static pages of proxy hosts). Run with plain LuaJIT
-- from the repository root (CI lua job):  luajit gateway/test/page_test.lua
package.path = "gateway/lua/?.lua;" .. package.path

local failed = 0
local function eq(got, want, what)
  if got ~= want then
    failed = failed + 1
    io.stderr:write(("FAIL %s\n  got:  %s\n  want: %s\n"):format(what, tostring(got), tostring(want)))
  end
end

-- minimal ngx stand-in: records what serve() sends
local sent
ngx = {
  HTTP_OK = 200, ERR = 4,
  var = { host = "shop.example.com" },
  log = function() end,
  print = function(s) sent.body = (sent.body or "") .. s end,
  exit = function(code) sent.exit = code end,
}
local function reset()
  sent = {}
  ngx.status = nil
  ngx.header = {}
end

local page = require("corvus.page")

local dir = os.getenv("TMPDIR") or os.getenv("TEMP") or "/tmp"
local function write(name, s)
  local p = dir .. "/" .. name
  local f = assert(io.open(p, "wb"))
  f:write(s)
  f:close()
  return p
end

-- render: the marker is replaced (every occurrence), the host HTML-escaped
eq(page.render("<p>{{HOST}}</p><title>{{HOST}}</title>", "a.example.com"), "<p>a.example.com</p><title>a.example.com</title>", "marker replaced")
eq(page.render("<p>{{HOST}}</p>", "<x\"&'>"), "<p>&lt;x&quot;&amp;&#39;&gt;</p>", "host escaped")
eq(page.render("<p>{{HOST}}</p>", "%1"), "<p>%1</p>", "no pattern expansion in the host")
eq(page.render("none", nil), "none", "no marker")

-- serve a built-in page with status 503: no-store, host filled in
local p1 = write("corvus_page_test_1.html", "<h1>Down</h1><div>{{HOST}}</div>")
reset()
page.serve(p1, 503, true)
eq(ngx.status, 503, "status 503")
eq(ngx.header["Content-Type"], "text/html; charset=utf-8", "content type")
eq(ngx.header["Cache-Control"], "no-store", "503 not cached")
eq(sent.body, "<h1>Down</h1><div>shop.example.com</div>", "body with host")
eq(ngx.header["Content-Length"], #sent.body, "content length")
eq(sent.exit, 200, "request finalized")

-- custom HTML (has_host false) is served byte for byte, also {{HOST}} and binary-ish bytes
local custom = "<!doctype html>\r\n<p>{{HOST}} {{x}} \226\156\147</p>\n"
local p2 = write("corvus_page_test_2.html", custom)
reset()
page.serve(p2, 200)
eq(ngx.status, 200, "status 200")
eq(ngx.header["Cache-Control"], nil, "200 without Cache-Control")
eq(sent.body, custom, "custom HTML exact")

-- cached: a changed file is not read again within the same path (new versions have new paths)
write("corvus_page_test_2.html", "changed")
reset()
page.serve(p2, 410)
eq(sent.body, custom, "cached per path")
eq(ngx.header["Cache-Control"], "no-store", "410 not cached")

-- missing file: 500, nothing sent
reset()
page.serve(dir .. "/corvus_page_test_missing.html", 200)
eq(sent.exit, 500, "missing file answers 500")
eq(sent.body, nil, "no body for a missing file")

os.remove(p1)
os.remove(p2)
if failed > 0 then
  io.stderr:write(failed .. " test(s) failed\n")
  os.exit(1)
end
print("page_test: ok")
