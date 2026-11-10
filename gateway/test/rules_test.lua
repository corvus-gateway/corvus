-- Unit tests for condition evaluation in corvus/rules.lua: regex conditions whose
-- evaluation fails. Run with plain LuaJIT from the repository root (CI lua job):
--   luajit gateway/test/rules_test.lua
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local eq = T.eq

-- only condition evaluation is tested: the other modules are not used
for _, m in ipairs({ "cjson.safe", "corvus.body", "corvus.ualist", "corvus.challenge",
  "corvus.underattack", "corvus.accesslog", "corvus.ipkey" }) do
  package.loaded[m] = {}
end
-- pattern "ERR" stands for a regex PCRE fails to evaluate (e.g. match limit exceeded);
-- (compiles fine, fails on a subject); other patterns are plain substrings
ngx.re = { find = function(s, p)
  if p == "ERR" and s ~= "" then return nil, nil, "pcre_exec() failed: -8" end
  local a, b = s:find(p, 1, true)
  return a, b
end }
local logged = 0
ngx.log = function() logged = logged + 1 end

local rules = require("corvus.rules")
local compile, test = rules._compile_cond, rules._test

local function cond(op, pattern, action)
  local c = compile({ f = "http.path", op = op, v = { pattern } })
  c.on_error = action == "block" or action == "challenge"
  return c
end
local function run(c, path)
  ngx.var = { uri = path }
  return test({}, c, {})
end

eq(run(cond("matches", "adm", "block"), "/admin"), true, "matches")
eq(run(cond("matches", "adm", "block"), "/home"), false, "no match")
eq(run(cond("not_matches", "adm", "block"), "/home"), true, "not_matches")

-- evaluation errors: block and challenge rules fire, allow and log rules do not
eq(run(cond("matches", "ERR", "block"), "/x"), true, "error: block rule, matches")
eq(run(cond("not_matches", "ERR", "block"), "/x"), true, "error: block rule, not_matches")
eq(run(cond("matches", "ERR", "challenge"), "/x"), true, "error: challenge rule")
eq(run(cond("matches", "ERR", "allow"), "/x"), false, "error: allow rule, matches")
eq(run(cond("not_matches", "ERR", "allow"), "/x"), false, "error: allow rule, not_matches")
eq(run(cond("matches", "ERR", "log"), "/x"), false, "error: log rule")

-- logged at most once a minute
logged = 0
T.set_time(1800000000)
run(cond("matches", "ERR", "block"), "/x")
run(cond("matches", "ERR", "block"), "/x")
eq(logged, 1, "error logged once")
T.set_time(1800000061)
run(cond("matches", "ERR", "block"), "/x")
eq(logged, 2, "logged again after a minute")

T.done("rules_test")
