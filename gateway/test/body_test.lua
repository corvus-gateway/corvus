-- Unit tests for corvus/body.lua (normalisation, multipart and charset handling, limits).
-- Run with plain LuaJIT from the repository root (CI lua job):
--   luajit gateway/test/body_test.lua
-- cjson and ngx.re are OpenResty modules: a small JSON decoder stands in for cjson, and
-- signatures are matched as plain substrings.
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local eq = T.eq

-- ---------------------------------------------------------------- stand-ins
local function json_decode(s, max_depth)
  local pos, depth = 1, 0
  local function ws() pos = s:find("[^ \t\r\n]", pos) or #s + 1 end
  local value
  local function str()
    local out = {}
    pos = pos + 1
    while true do
      local c = s:sub(pos, pos)
      if c == "" then error("unterminated string") end
      if c == '"' then pos = pos + 1 break end
      if c == "\\" then
        local e = s:sub(pos + 1, pos + 1)
        if e == "u" then
          local n = tonumber(s:sub(pos + 2, pos + 5), 16)
          out[#out + 1] = n < 128 and string.char(n) or "?"
          pos = pos + 6
        else
          out[#out + 1] = ({ n = "\n", r = "\r", t = "\t", b = "\b", f = "\f" })[e] or e
          pos = pos + 2
        end
      else
        out[#out + 1] = c
        pos = pos + 1
      end
    end
    return table.concat(out)
  end
  value = function()
    ws()
    local c = s:sub(pos, pos)
    if c == "{" or c == "[" then
      depth = depth + 1
      if depth > max_depth then error("Found too many nested data structures") end
      local t, close = {}, c == "{" and "}" or "]"
      pos = pos + 1
      ws()
      if s:sub(pos, pos) == close then pos = pos + 1 depth = depth - 1 return t end
      while true do
        ws()
        if c == "{" then
          local k = str()
          ws()
          if s:sub(pos, pos) ~= ":" then error("expected colon") end
          pos = pos + 1
          t[k] = value()
        else
          t[#t + 1] = value()
        end
        ws()
        local d = s:sub(pos, pos)
        pos = pos + 1
        if d == close then break end
        if d ~= "," then error("expected comma") end
      end
      depth = depth - 1
      return t
    elseif c == '"' then return str()
    elseif s:sub(pos, pos + 3) == "true" then pos = pos + 4 return true
    elseif s:sub(pos, pos + 4) == "false" then pos = pos + 5 return false
    elseif s:sub(pos, pos + 3) == "null" then pos = pos + 4 return false
    end
    local a, b = s:find("^-?%d+%.?%d*[eE]?[-+]?%d*", pos)
    if not a then error("unexpected character") end
    pos = b + 1
    return tonumber(s:sub(a, b))
  end
  local v = value()
  ws()
  if pos <= #s then error("trailing data") end
  return v
end

package.loaded["cjson.safe"] = { new = function()
  local depth = 1000
  return {
    decode_max_depth = function(d) depth = d end,
    decode = function(s)
      local ok, v = pcall(json_decode, s, depth)
      if ok then return v end
      return nil, tostring(v)
    end,
  }
end }

ngx.re = { find = function(s, p) return (s:find(p, 1, true)) end }
ngx.unescape_uri = function(s)
  return (s:gsub("%%(%x%x)", function(h) return string.char(tonumber(h, 16)) end))
end
local REQ = { body = "", version = 1.1 }
ngx.req = {
  http_version = function() return REQ.version end,
  read_body = function() end,
  get_body_data = function() return REQ.body end,
  get_body_file = function() return nil end,
}

local body = require("corvus.body")
local norm, params = body._normalise, body._parse_params
body.set_signatures({ { id = "marker", pattern = "EVIL" } })

local function inspect(b, ct)
  ngx.ctx = {}
  REQ.body = b
  ngx.var = { http_content_length = tostring(#b), content_length = tostring(#b), content_type = ct }
  return body.inspect(1048576)
end

local function lines(text)
  local t = {}
  for l in (text .. "\n"):gmatch("(.-)\n") do t[l] = true end
  return t
end

-- ---------------------------------------------------------------- basics
eq(inspect('{"a":"EVIL"}', "application/json"), "marker", "json value")
eq(inspect('{"a":"fine"}', "application/json"), nil, "clean json")
eq(inspect("a=EV%49L", "application/x-www-form-urlencoded"), "marker", "form value decoded")
eq(inspect("", "multipart/form-data"), nil, "empty body with a multipart type is fine")

-- ---------------------------------------------------------------- item limit
local N = body.MAX_ITEMS
local pad = {}
for i = 1, N + 10 do pad[i] = '"x"' end
local big = '[' .. table.concat(pad, ",") .. ',"EVIL"]'
eq(inspect(big, "application/json"), "items-overflow", "array padded past the item limit")
local _, meta = norm(big, "application/json")
eq(meta.truncated, true, "padded array marked truncated")
local formpad = string.rep("a=b&", N + 10) .. "z=EVIL"
eq(inspect(formpad, "application/x-www-form-urlencoded"), "items-overflow", "form padded past the item limit")
local nested = 'v=' .. ngx.unescape_uri('{"a":"{\\"b\\":\\"{\\\\\\"c\\\\\\":1}\\"}"}')
_, meta = norm(nested, "application/x-www-form-urlencoded")
eq(meta.truncated, true, "JSON in strings nested past the limit")
local deep = string.rep("[", 70) .. string.rep("]", 70)
_, meta = norm(deep, "application/json")
eq(meta.truncated, true, "JSON nested deeper than the decoder accepts")
_, meta = norm('{"a":"[draft] title"}', "application/json")
eq(meta.truncated, nil, "bracketed text is not nested JSON")
-- an empty array/object at the JSON-in-string limit holds nothing left uninspected
local function in_strings(inner) -- inner JSON as a string value, three levels down
  local l3 = '{"c":' .. string.format("%q", inner) .. '}'
  local l2 = '{"b":' .. string.format("%q", l3) .. '}'
  return '{"a":' .. string.format("%q", l2) .. '}'
end
for _, empty in ipairs({ "[]", "{}", " [ ] " }) do
  _, meta = norm(in_strings(empty), "application/json")
  eq(meta.truncated, nil, "empty " .. empty .. " at the limit")
end
_, meta = norm(in_strings('{"d":1}'), "application/json")
eq(meta.truncated, true, "non-empty object at the limit")
_, meta = norm(in_strings('[1]'), "application/json")
eq(meta.truncated, true, "non-empty array at the limit")
for k, v in pairs(body.UNINSPECTED) do eq(v, true, "uninspected id " .. k) end

-- ---------------------------------------------------------------- charset / NUL
eq(inspect('{"a":"b"}', "application/json; charset=utf-16"), "unsupported-charset", "utf-16 json")
eq(inspect('{"a":"b"}', 'application/json; charset="UTF-8"'), nil, "quoted utf-8")
eq(inspect('a=b', "application/x-www-form-urlencoded; charset=ISO-8859-1"), nil, "latin-1 form")
eq(inspect('a=b', "application/x-www-form-urlencoded; charset=utf-7"), "unsupported-charset", "utf-7 form")
-- ASCII-compatible legacy charsets are read as bytes
for _, cs in ipairs({ "EUC-KR", "cp949", "Shift_JIS", "GBK", "gb18030", "Big5", "latin1", "cp1252", "windows-1251", "koi8-r" }) do
  eq(inspect('a=%B0%A1&b=EVI', "application/x-www-form-urlencoded; charset=" .. cs), nil, cs .. " form")
  eq(inspect('{"a":"\176\161"}', "application/json; charset=" .. cs), nil, cs .. " json")
end
eq(inspect('a=EVIL', "application/x-www-form-urlencoded; charset=euc-kr"), "marker", "euc-kr form is inspected")
eq(inspect('{"a":"EVIL"}', "application/json; charset=euc-kr"), "marker", "euc-kr json is inspected")
-- charsets that are not ASCII-compatible
for _, cs in ipairs({ "UTF-16", "utf-16le", "UTF-16BE", "utf16", "utf-32", "UTF-32LE", "ucs-2", "UCS-4", "utf-7",
  "ISO-10646-UCS-2", "unicode", "ebcdic-cp-us", "IBM037", "cp037", "ibm01140", "cp1140", "IBM1047", "cp500", "csIBM037" }) do
  eq(inspect('{"a":"b"}', "application/json; charset=" .. cs), "unsupported-charset", cs .. " json")
end
eq(inspect('a=b', "application/x-www-form-urlencoded; charset=utf-16"), "unsupported-charset", "utf-16 form")
for _, cs in ipairs({ "ibm437", "cp850", "cp1250", "ibm866" }) do -- ASCII-compatible code pages
  eq(inspect('{"a":"b"}', "application/json; charset=" .. cs), nil, cs .. " json")
end
eq(inspect('{\0"\0a\0"\0:\0"\0b\0"\0}\0', "application/json"), "unsupported-charset", "NUL bytes in json")
eq(inspect('\255\254{', "application/json"), "unsupported-charset", "UTF-16 byte order mark")
eq(inspect('{"a":"EVIL"}', "application/json; charset=utf-16"), "marker", "a signature match wins")
eq(inspect('x\0y', "application/octet-stream"), nil, "NUL bytes in other types are fine")

-- ---------------------------------------------------------------- content type parameters
local mt, ps = params('multipart/form-data; boundary="a;b\\"c"; x=1')
eq(mt, "multipart/form-data", "media type")
eq(ps[1][1] .. "=" .. ps[1][2], 'boundary=a;b"c', "quoted boundary with ; and quoted pair")
eq(ps[2][2], "1", "token parameter")
eq(params('text/plain; a="unterminated'), nil, "unterminated quoted string")
eq(params('text/plain; a=b c'), nil, "junk after a value")
eq(inspect('{"a":"EVIL"}', "application/json; x=multipart/form-data"), "marker", "media type is not matched as a substring")

-- ---------------------------------------------------------------- multipart
local function mp(b, parts, close)
  local out = {}
  for _, p in ipairs(parts) do out[#out + 1] = "--" .. b .. "\r\n" .. p .. "\r\n" end
  if close ~= false then out[#out + 1] = "--" .. b .. "--\r\n" end
  return table.concat(out)
end
local field = 'Content-Disposition: form-data; name="f"\r\n\r\nEVIL'
local MP = "multipart/form-data; boundary="

eq(inspect(mp("XX", { field }), MP .. "XX"), "marker", "plain multipart")
eq(inspect(mp("XX", { field }), MP .. '"XX"'), "marker", "quoted boundary")
-- only the parameter named "boundary" counts
eq(inspect(mp("REAL", { field }), MP .. 'REAL; xboundary="FAKE"'), "marker", "xboundary is another parameter")
eq(inspect(mp("REAL", { field }), MP .. 'REAL; boundary="FAKE"'), "multipart-anomaly", "repeated boundary")
eq(inspect(mp("XX", { field }), "multipart/form-data"), "multipart-anomaly", "missing boundary")
eq(inspect(mp("XX", { field }), MP .. "YY"), "multipart-anomaly", "boundary never appears")
eq(inspect("--XX--\r\n", MP .. "XX"), nil, "empty form (close delimiter only)")
eq(inspect("--XX--", MP .. "XX"), nil, "empty form without a line break")
local _, m0 = norm("--XX--\r\n", MP .. "XX")
eq(m0.parts, 0, "empty form has no parts")
eq(m0.anomaly, nil, "empty form is not an anomaly")
eq(inspect("EVIL", MP .. "XX"), "multipart-anomaly", "no delimiter")
eq(inspect(mp("XX", { field }, false), MP .. "XX"), "multipart-anomaly", "no close delimiter")
eq(inspect(mp("XX", { 'Content-Disposition: form-data; name="f"\r\nEVIL' }), MP .. "XX"), "multipart-anomaly", "part without a header section")
-- a delimiter must start a line
local inline = "--XX\r\n" .. 'Content-Disposition: form-data; name="f"\r\n\r\nkeep --XX\r\nEVIL\r\n--XX--\r\n'
local text = norm(inline, MP .. "XX")
eq(lines(text)["keep --XX  EVIL"], true, "delimiter inside a value is content")
-- the boundary followed by other text is not a delimiter
text = norm("--XX\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\na\r\n--XXY\r\nb\r\n--XX--", MP .. "XX")
eq(lines(text)["a  --XXY  b"], true, "longer boundary-like line is content")
-- preamble before the first delimiter is skipped
eq(inspect("EVIL preamble\r\n" .. mp("XX", { 'Content-Disposition: form-data; name="f"\r\n\r\nok' }), MP .. "XX"), nil, "preamble ignored")
-- header section ends at the first empty line, LF or CRLF
text = norm("--XX\r\nContent-Disposition: form-data; name=\"f\"\n\nv1\r\n\r\nv2\r\n--XX--\r\n", MP .. "XX")
eq(lines(text)["v1    v2"], true, "LF empty line ends the headers")
text = norm("--XX\r\n\r\nraw\r\n--XX--\r\n", MP .. "XX")
eq(lines(text)["raw"], true, "part without headers")
-- name and filename, quoted or not
text = norm(mp("XX", { "Content-Disposition: form-data; name=__proto__\r\n\r\nv" }), MP .. "XX")
eq(lines(text)["__proto__"], true, "unquoted name")
eq(lines(text)["v"], true, "value of an unquoted name")
text = norm(mp("XX", { "Content-Disposition: form-data; name=f; filename=a.txt\r\n\r\nEVIL" }), MP .. "XX")
eq(lines(text)["a.txt"], true, "unquoted filename")
eq(lines(text)["EVIL"], nil, "file contents are skipped")
text = norm(mp("XX", { 'Content-Disposition: form-data; xfilename="n"; name="f"\r\n\r\nEVIL' }), MP .. "XX")
eq(lines(text)["EVIL"], true, "xfilename is not a filename")
-- browsers do not escape backslashes in file names
text = norm(mp("XX", { 'Content-Disposition: form-data; name="f"; filename="C:\\Users\\me\\a.txt"\r\n\r\nEVIL' }), MP .. "XX")
eq(lines(text)["C:\\Users\\me\\a.txt"], true, "Windows path kept as sent")
eq(inspect(mp("XX", { 'Content-Disposition: form-data; name="f"; filename="dir\\"\r\n\r\nv' }), MP .. "XX"), nil, "filename ending with a backslash")
text = norm(mp("XX", { 'Content-Disposition: form-data; name="f"; filename="dir\\"\r\n\r\nv' }), MP .. "XX")
eq(lines(text)["dir\\"], true, "trailing backslash kept")
text = norm(mp("XX", { "Content-Disposition: form-data; name=f; filename= my file.txt \r\n\r\nEVIL" }), MP .. "XX")
eq(lines(text)["my file.txt"], true, "unquoted filename with a space")
eq(lines(text)["EVIL"], nil, "file contents of an unquoted filename are skipped")
local _, cdp = params('form-data; name="a\\"; filename=x y', true)
eq(cdp[1][2] .. "|" .. cdp[2][2], "a\\|x y", "disposition parameters")
eq(inspect(mp("XX", { 'Content-Disposition: form-data; name="a"; name="b"\r\n\r\nv' }), MP .. "XX"), "multipart-anomaly", "repeated name")
eq(inspect(mp("XX", { 'Content-Disposition: form-data; name="a"\r\nContent-Disposition: form-data; name="b"\r\n\r\nv' }), MP .. "XX"), "multipart-anomaly", "repeated Content-Disposition")
eq(inspect(mp("XX", { 'Content-Disposition: form-data; name="f"\r\n\r\n{"k":"EVIL"}' }), MP .. "XX"), "marker", "JSON field value decoded")

-- ---------------------------------------------------------------- read errors
ngx.ctx = {}
ngx.var = { http_content_length = "10", content_length = "2000000", content_type = "application/json" }
eq(body.inspect(1048576), "skip:too_large", "too large body is skipped")
ngx.ctx = {}
ngx.var = { http_content_length = "10", content_length = "10", content_type = "application/json" }
local saved = ngx.req.get_body_data
ngx.req.get_body_data = function() return nil end
ngx.req.get_body_file = function() return "/nonexistent/corvus-body" end
eq(body.inspect(1048576), "unreadable", "unreadable body")
ngx.req.get_body_data = saved
ngx.ctx = {}
ngx.var = { http_content_length = "10", content_length = "10", content_type = "application/json", http_content_encoding = "gzip" }
eq(body.inspect(1048576), "encoded", "compressed body")

T.done("body_test")
