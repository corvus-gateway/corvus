-- Unit tests for corvus/ualist.lua. Run with plain LuaJIT from the repository root
-- (CI lua job):  luajit gateway/test/ualist_test.lua
package.path = "gateway/lua/?.lua;" .. package.path
local ualist = require("corvus.ualist")

local failed = 0
local function eq(got, want, what)
  if got ~= want then
    failed = failed + 1
    io.stderr:write(("FAIL %s\n  got:  %s\n  want: %s\n"):format(what, tostring(got), tostring(want)))
  end
end

local m = ualist.new({ "AhrefsBot", "indy library", "1h4x.com", "sqlmap/", "", 42 })
eq(#m, 4, "empty and non-string entries dropped")
local function hit(ua) return ualist.match(m, ua and ua:lower()) end
eq(hit("Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)"), true, "substring, case-insensitive")
eq(hit("Mozilla/4.0 (compatible; Indy Library)"), true, "entry with a space")
eq(hit("sqlmap/1.7.2#stable (https://sqlmap.org)"), true, "entry with a slash")
eq(hit("Mozilla/5.0 (X11; Linux x86_64) Firefox/120.0"), false, "ordinary browser")
-- "." is literal, not a pattern: "1h4x.com" must not match "1h4xxcom".
eq(hit("1h4xxcom"), false, "dot is literal")
eq(hit("visit 1H4X.COM now"), true, "dot entry matches literally")
-- Pattern characters in the User-Agent are harmless.
eq(hit("%a+.*[(]"), false, "pattern characters in the User-Agent")
eq(hit(""), false, "empty User-Agent")
eq(hit(nil), false, "missing User-Agent")
eq(ualist.match(ualist.new({}), "anything"), false, "empty list")
eq(ualist.match(ualist.new(nil), "anything"), false, "nil list")
local p = ualist.new({ "a%d+" })
eq(ualist.match(p, "a123"), false, "entries are not Lua patterns")
eq(ualist.match(p, "xa%d+y"), true, "entries match literally")

if failed > 0 then
  io.stderr:write(failed .. " test(s) failed\n")
  os.exit(1)
end
print("ualist_test: ok")
