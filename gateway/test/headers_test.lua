-- Unit tests for the pure parts of corvus/headers.lua. Run with plain LuaJIT from the
-- repository root (CI lua job):  luajit gateway/test/headers_test.lua
package.path = "gateway/lua/?.lua;" .. package.path
local headers = require("corvus.headers")
local flags = headers.cookie_flags

local failed = 0
local function eq(got, want, what)
  if got ~= want then
    failed = failed + 1
    io.stderr:write(("FAIL %s\n  got:  %s\n  want: %s\n"):format(what, tostring(got), tostring(want)))
  end
end

eq(flags("sid=abc; Path=/", false), "sid=abc; Path=/; HttpOnly; SameSite=Lax", "plain cookie over HTTP")
eq(flags("sid=abc; Path=/", true), "sid=abc; Path=/; HttpOnly; SameSite=Lax; Secure", "plain cookie over HTTPS")
eq(flags("sid=abc", true), "sid=abc; HttpOnly; SameSite=Lax; Secure", "bare cookie")
eq(flags("sid=abc; Path=/;", false), "sid=abc; Path=/; HttpOnly; SameSite=Lax", "trailing semicolon")
eq(flags("sid=abc; Secure; HttpOnly; SameSite=Strict", true), "sid=abc; Secure; HttpOnly; SameSite=Strict", "complete cookie untouched")
eq(flags("sid=abc; secure; httponly; samesite=none", true), "sid=abc; secure; httponly; samesite=none", "case-insensitive attributes")
eq(flags("sid=abc;HTTPONLY", true), "sid=abc;HTTPONLY; SameSite=Lax; Secure", "keeps existing HttpOnly without space")
eq(flags("sid=abc; SameSite = None", true), "sid=abc; SameSite = None; HttpOnly; Secure", "SameSite with spaces")
-- The cookie's own name is not an attribute: a cookie called "HttpOnly" still gets the flag.
eq(flags("HttpOnly=1", false), "HttpOnly=1; HttpOnly; SameSite=Lax", "cookie named like an attribute")
-- Commas in Expires must not split the cookie.
eq(flags("a=1; Expires=Wed, 21 Oct 2026 07:28:00 GMT", false), "a=1; Expires=Wed, 21 Oct 2026 07:28:00 GMT; HttpOnly; SameSite=Lax", "Expires with comma")
eq(flags("", true), "", "empty value")
eq(flags(nil, true), nil, "nil value")

if failed > 0 then
  io.stderr:write(failed .. " test(s) failed\n")
  os.exit(1)
end
print("headers_test: ok")
