-- Corvus request body inspection ("virtual patching" for body-borne exploits).
--
-- Reads the request body (bounded), normalises it by content type and matches a small
-- set of high-signal signatures. Normalisation defeats the usual wrapping tricks:
--   JSON        → decoded; keys and string values (nested), JSON inside string values too
--   urlencoded  → keys/values percent-decoded (twice, for double encoding)
--   multipart   → field names, file names and non-file values (file contents are skipped);
--                 values that look like JSON are decoded as well
--   other       → raw text (JSON-looking bodies are decoded)
-- The normalised text is one item per line, so signatures can anchor on whole keys/values.
-- Bodies above the host limit are not read (logged as "skip"). Bodies that are read but
-- cannot be inspected completely are reported with their own ids (see UNINSPECTED):
-- compressed bodies (Content-Encoding), bodies whose normalisation hit a limit, and
-- JSON/form bodies in a charset that is not ASCII-compatible (UTF-16/32, UTF-7, EBCDIC)
-- or with NUL bytes.

local cjson_new = require("cjson.safe").new
local find, lower, sub, byte, gsub = string.find, string.lower, string.sub, string.byte, string.gsub
local re_find = ngx.re.find
local unescape = ngx.unescape_uri
local concat = table.concat

local _M = {}

local json = cjson_new()
json.decode_max_depth(64)

local MAX_ITEMS = 100000         -- normalised items per request
local MAX_PARTS = 1000           -- multipart parts before "anomaly"
local MAX_BRACKETS = 32          -- PHP array nesting in a field name before "anomaly"
local MAX_JSON_IN_STRING = 3     -- levels of JSON decoded inside string values

-- Results of inspect() for bodies that were read but could not be inspected completely.
-- Hosts in block mode refuse them; hosts that only log record them as not inspected.
_M.UNINSPECTED = {
  ["encoded"] = true,              -- Content-Encoding (compressed body)
  ["unreadable"] = true,           -- buffered body file could not be read
  ["items-overflow"] = true,       -- normalisation stopped at a limit
  ["unsupported-charset"] = true,  -- JSON/form body in a charset the decoders cannot read
}

-- High-signal signatures (id, PCRE, options). Matched against the normalised text.
-- Keep them specific: this is virtual patching, not a generic WAF.
local SIGNATURES = {
  -- React Server Components / Next.js Server Actions RCE (CVE-2025-55182 "React2Shell")
  { "react2shell", [=[\$@?\d*:(?:__proto__|constructor|prototype)(?::|$)|^resolved_model$|^\$@\d+$]=], "joim" },
  -- JavaScript prototype pollution via request keys/paths (Node.js apps)
  { "proto-pollution", [=[^__proto__$|__proto__(?:\]|\.|\[)|constructor(?:\]?\[|\.)prototype]=], "joim" },
  -- PHP object injection (serialized objects) and common gadget-chain classes
  { "php-object", [=[(?:^|[^a-z0-9_])[oc]:\d+:"[a-z_\\][a-z0-9_\\]*":\d+:\{]=], "joim" },
  { "php-gadget", [=[(?:illuminate|monolog|guzzlehttp|symfony|laminas|doctrine|phpggc)\\+[a-z]+\\+[a-z]]=], "joim" },
  -- Craft CMS RCE (CVE-2025-32432): "__class" object instantiation
  { "craft-class", [=[^__class$]=], "joim" },
  -- PHP session upload-progress abuse (php2shell CVE-2026-109618/109619 vector)
  { "php-upload-progress", [=[^php_session_upload_progress$]=], "joim" },
  -- Log4Shell-style lookups
  { "log4shell", [=[\$\{(?:jndi|lower:|upper:|env:|sys:|\$\{)]=], "joim" },
  -- Java serialized objects (base64 / raw magic)
  { "java-deser", [=[(?:^|[^A-Za-z0-9+/])rO0AB[A-Za-z0-9+/]{8}|\xac\xed\x00\x05]=], "jom" },
}

-- ------------------------------------------------------------------ read

-- may_have_body: HTTP/1 announces a body with Content-Length or Transfer-Encoding. HTTP/2
-- and HTTP/3 have no Transfer-Encoding and Content-Length is optional there, so a request
-- without it may still carry a body (streamed uploads, fetch with a stream body).
local function may_have_body()
  local cl = tonumber(ngx.var.http_content_length or "")
  if cl then return cl > 0 end
  if (ngx.var.http_transfer_encoding or "") ~= "" then return true end
  return (ngx.req.http_version() or 1) >= 2
end

-- Returns the raw body (possibly ""), or nil + reason ("too_large", "unreadable").
local function read_raw(max)
  if not may_have_body() then return "" end
  local cl = tonumber(ngx.var.content_length)
  if cl and cl > max then return nil, "too_large" end
  ngx.req.read_body()
  local data = ngx.req.get_body_data()
  if not data then
    local path = ngx.req.get_body_file()
    if not path then return "" end
    local f = io.open(path, "rb")
    if not f then return nil, "unreadable" end
    data = f:read(max + 1)
    f:close()
    data = data or ""
  end
  if #data > max then return nil, "too_large" end
  return data
end

-- ------------------------------------------------------------------ normalise

-- Items are collected in acc (acc.n = count). When a limit stops the walk (item count,
-- nesting depth), acc.truncated is set: the body was only partly inspected.
local function add(acc, v)
  if acc.n >= MAX_ITEMS then acc.truncated = true return end
  acc.n = acc.n + 1
  acc[acc.n] = v
end

local flatten -- forward

local function looks_json(s)
  local b = byte(s, (find(s, "%S")) or 1)
  return b == 123 or b == 91 -- { or [
end

-- decode_json: the decoded value, or nil. A document nested deeper than the decoder
-- accepts marks the body as only partly inspected.
local function decode_json(acc, s)
  local ok, obj, err = pcall(json.decode, s)
  if not ok then return nil end
  if obj == nil and type(err) == "string" and find(err, "nested", 1, true) then
    acc.truncated = true
  end
  return obj
end

local function add_value(acc, v, depth)
  add(acc, v)
  if #v > 1 and looks_json(v) then
    local obj = decode_json(acc, v)
    if type(obj) == "table" then
      -- JSON nested in string values deeper than this is not walked ("[]" and "{}" hold
      -- nothing to walk)
      if depth >= MAX_JSON_IN_STRING then
        if next(obj) ~= nil then acc.truncated = true end
        return
      end
      flatten(acc, obj, depth + 1)
    end
  end
end

flatten = function(acc, obj, depth)
  for k, v in pairs(obj) do
    if acc.n >= MAX_ITEMS then acc.truncated = true return end
    if type(k) == "string" then add(acc, k) end
    local t = type(v)
    if t == "string" then add_value(acc, v, depth)
    elseif t == "table" then flatten(acc, v, depth)
    end
  end
end

local function decode_form(acc, s)
  for pair in s:gmatch("[^&]+") do
    if acc.n >= MAX_ITEMS then acc.truncated = true return end
    local k, v = pair:match("^([^=]*)=?(.*)$")
    for _, x in ipairs({ k, v }) do
      if x and x ~= "" then
        local d = unescape((gsub(x, "+", " ")))
        if find(d, "%", 1, true) then d = unescape(d) end
        add_value(acc, d, 1)
      end
    end
  end
end

local function trim(s) return (gsub(s, "^%s*(.-)%s*$", "%1")) end

-- Header parameters (RFC 9110, RFC 6266): `; name=token` or `; name="quoted \" string"`.
-- Returns the media type / disposition type (lower case) and a list of { name, value }
-- (names lower case, values as sent), or nil when the parameter list is malformed.
-- disposition (Content-Disposition of a multipart part): browsers send file names as
-- they are (HTML: only CR, LF and '"' are percent-encoded), so a backslash is literal
-- in a quoted value, and an unquoted value runs to the next ";" (trimmed), as in
-- `filename=my file.txt`.
local function parse_params(h, disposition)
  local semi = find(h, ";", 1, true)
  local base = lower(trim(sub(h, 1, (semi or #h + 1) - 1)))
  local list = {}
  if not semi then return base, list end
  local pos, n = semi + 1, #h
  while true do
    local _, e = find(h, "^[%s;]*", pos)
    pos = e + 1
    if pos > n then break end
    local ns, ne = find(h, "^[%w!#%$%%&'%*%+%-%.%^_`|~]+", pos)
    if not ns then return nil end
    local name = lower(sub(h, ns, ne))
    _, e = find(h, "^%s*=%s*", ne + 1)
    if not e then return nil end
    pos = e + 1
    local value
    if byte(h, pos) == 34 then -- quoted string
      local buf, i = {}, pos + 1
      while true do
        local c = byte(h, i)
        if not c then return nil end            -- unterminated
        if c == 34 then break end
        if c == 92 and i < n and not disposition then i = i + 1 end -- quoted pair
        buf[#buf + 1] = sub(h, i, i)
        i = i + 1
      end
      value, pos = concat(buf), i + 1
    elseif disposition then
      local vs, ve = find(h, "^[^;]*", pos)
      value, pos = trim(sub(h, vs, ve)), ve + 1
    else
      local vs, ve = find(h, "^[^;%s]*", pos)
      value, pos = sub(h, vs, ve), ve + 1
    end
    _, e = find(h, "^%s*", pos)
    pos = e + 1
    if pos <= n and byte(h, pos) ~= 59 then return nil end -- anything but ";" after a value
    list[#list + 1] = { name, value }
  end
  return base, list
end

-- param: the value of the named parameter, and true when it appears more than once.
local function param(list, name)
  local v, dup = nil, false
  for _, p in ipairs(list) do
    if p[1] == name then
      if v ~= nil then dup = true else v = p[2] end
    end
  end
  return v, dup
end

-- next_delim finds the next "--boundary" delimiter line at or after pos. A delimiter
-- starts the body or a line and is followed by "--" (close delimiter) or by optional
-- spaces and a line break. Returns the start of the delimiter (including the line break
-- before it, which belongs to the delimiter), the position after the delimiter line,
-- and whether it is the close delimiter.
local function next_delim(s, delim, pos)
  while true do
    local a, b
    if pos == 1 and sub(s, 1, #delim) == delim then
      a, b = 1, #delim
    else
      local nl = find(s, "\n" .. delim, pos, true)
      if not nl then return nil end
      a, b = nl, nl + #delim
      if a > 1 and byte(s, a - 1) == 13 then a = a - 1 end -- CRLF
    end
    if sub(s, b + 1, b + 2) == "--" then return a, #s + 1, true end
    local _, le = find(s, "^[ \t]*\r?\n", b + 1)
    if le then return a, le + 1, false end
    pos = b + 1 -- the boundary followed by other text is content, not a delimiter
  end
end

local CD_HEADER = "^[Cc][Oo][Nn][Tt][Ee][Nn][Tt]%-[Dd][Ii][Ss][Pp][Oo][Ss][Ii][Tt][Ii][Oo][Nn]%s*:(.*)$"

-- Multipart: sets meta.anomaly when the structure cannot be parsed unambiguously
-- (missing, empty or repeated boundary parameter, no delimiter, no close delimiter, a
-- part without a header section, repeated or malformed Content-Disposition parameters).
-- A body that starts with the close delimiter has no parts (an empty form).
local function decode_multipart(acc, s, list, meta)
  local boundary, dup = param(list or {}, "boundary")
  if not list or not boundary or boundary == "" or dup then
    meta.anomaly = true
    add(acc, s)
    return
  end
  local delim = "--" .. boundary
  local _, start, close = next_delim(s, delim, 1)
  if not start then meta.anomaly = true add(acc, s) return end
  if close then meta.parts = 0 return end
  local parts = 0
  while true do
    local a, nstart, nclose = next_delim(s, delim, start)
    if not a then meta.anomaly = true end -- no close delimiter: the rest is one part
    local part = sub(s, start, (a or #s + 1) - 1)
    parts = parts + 1
    if parts > MAX_PARTS then meta.anomaly = true break end
    -- the header section ends at the first empty line (CRLF or LF, whichever is first);
    -- a part that starts with the empty line has no headers
    local hs, he = find("\n" .. part, "\n\r?\n")
    if not hs then
      meta.anomaly = true
      add(acc, part)
    else
      local cd, ncd = nil, 0
      for line in sub(part, 1, hs - 1):gmatch("[^\r\n]+") do
        local v = line:match(CD_HEADER)
        if v then cd, ncd = v, ncd + 1 end
      end
      local name, filename
      if cd then
        local _, ps = parse_params(cd, true)
        if not ps or ncd > 1 then meta.anomaly = true end
        ps = ps or {}
        local d1, d2, d3, fnx
        name, d1 = param(ps, "name")
        filename, d2 = param(ps, "filename")
        fnx, d3 = param(ps, "filename*")
        if d1 or d2 or d3 then meta.anomaly = true end
        if fnx then
          add(acc, fnx)
          filename = filename or fnx
        end
      end
      if name then
        add(acc, name)
        local _, depth = gsub(name, "%[", "")
        if depth > MAX_BRACKETS then meta.anomaly = true end
      end
      if filename then
        add(acc, filename)                       -- file contents are not inspected
      else
        add_value(acc, sub(part, he), 1)
      end
    end
    if not a or nclose then break end
    start = nstart
  end
  meta.parts = parts
end

-- EBCDIC code pages (IBM/cp numbers): their bytes for JSON and form syntax are not ASCII.
local EBCDIC = {}
for _, n in ipairs({ 37, 273, 277, 278, 280, 284, 285, 290, 297, 420, 423, 424, 500, 870, 871,
  875, 880, 905, 918, 1025, 1026, 1047, 1140, 1141, 1142, 1143, 1144, 1145, 1146, 1147, 1148,
  1149 }) do
  EBCDIC[n] = true
end

-- foreign_charset: the charset does not keep ASCII bytes for ASCII characters, so the
-- decoders here would not read the body as the application does (UTF-16/32, UCS-2/4,
-- UTF-7, EBCDIC). ASCII-compatible charsets (UTF-8, EUC-KR, Shift_JIS, GBK, Big5,
-- ISO-8859-*, windows-125x ...) are read as bytes: the JSON and form syntax is ASCII.
local function foreign_charset(cs)
  cs = gsub(lower(trim(cs)), "[%-_%s]", "")
  if find(cs, "^utf16") or find(cs, "^utf32") or find(cs, "^ucs[24]") or find(cs, "^utf7")
    or find(cs, "^iso10646") or find(cs, "^csucs4") or find(cs, "^unicode")
    or find(cs, "^csunicode") or find(cs, "ebcdic", 1, true) then
    return true
  end
  local num = cs:match("^c?s?ibm(%d+)$") or cs:match("^cp(%d+)$")
  return num ~= nil and EBCDIC[tonumber(num)] == true
end

local function normalise(raw, ctype)
  local acc, meta = { n = 0 }, {}
  ctype = ctype or ""
  local mtype, list = parse_params(ctype)
  if not mtype then mtype = lower(trim(ctype:match("^[^;]*"))) end
  if raw == "" then return "", meta end
  local is_json = find(mtype, "json", 1, true) ~= nil
  local is_form = mtype == "application/x-www-form-urlencoded"
  if is_json or is_form then
    -- a body in an encoding that is not ASCII-compatible, or with NUL bytes, may be
    -- decoded differently by the application than here
    local cs = list and param(list, "charset")
    if (cs and foreign_charset(cs)) or find(raw, "\0", 1, true)
      or sub(raw, 1, 2) == "\254\255" or sub(raw, 1, 2) == "\255\254" then
      meta.charset = true
    end
  end
  if mtype == "multipart/form-data" then
    decode_multipart(acc, raw, list, meta)
  elseif is_json then
    local obj = decode_json(acc, raw)
    if type(obj) == "table" then flatten(acc, obj, 1)
    elseif type(obj) == "string" then add_value(acc, obj, 1)
    else add(acc, raw) end
  elseif is_form then
    decode_form(acc, raw)
  else
    add_value(acc, raw, 1)
  end
  if acc.truncated then meta.truncated = true end
  acc.n, acc.truncated = nil, nil
  -- one item per line; newlines inside values become spaces so ^/$ anchor whole items
  for i = 1, #acc do acc[i] = gsub(acc[i], "[\r\n]", " ") end
  return concat(acc, "\n"), meta
end

-- ------------------------------------------------------------------ API

-- text(max): normalised body text for rule conditions (cached per request).
-- Returns text or nil + reason.
function _M.text(max)
  local ctx = ngx.ctx
  if ctx.corvus_body_text ~= nil then
    if ctx.corvus_body_text == false then return nil, ctx.corvus_body_err end
    return ctx.corvus_body_text, nil, ctx.corvus_body_meta
  end
  local enc = ngx.var.http_content_encoding
  if may_have_body() and enc and enc ~= "" and lower(enc) ~= "identity" then
    ctx.corvus_body_text, ctx.corvus_body_err = false, "encoded"
    return nil, "encoded"
  end
  local raw, err = read_raw(max)
  if not raw then
    ctx.corvus_body_text, ctx.corvus_body_err = false, err
    return nil, err
  end
  local text, meta = normalise(raw, ngx.var.content_type)
  ctx.corvus_body_text, ctx.corvus_body_meta = text, meta
  return text, nil, meta
end

-- inspect(max): returns a signature id when the body matches, an id from UNINSPECTED
-- when the body was read but could not be inspected completely, "multipart-anomaly" for
-- a malformed multipart body, "skip:too_large" for a body above the limit, or nil.
function _M.inspect(max)
  local text, err, meta = _M.text(max)
  if not text then
    if err == "encoded" or err == "unreadable" then return err end
    return "skip:" .. tostring(err)
  end
  if meta and meta.anomaly then return "multipart-anomaly" end
  if text ~= "" then
    for _, s in ipairs(SIGNATURES) do
      local from = re_find(text, s[2], s[3])
      if from then return s[1] end
    end
  end
  if meta and meta.charset then return "unsupported-charset" end
  if meta and meta.truncated then return "items-overflow" end
  return nil
end

-- set_signatures replaces the signature list with the active rule bundle's
-- ({id, pattern, options}); invalid patterns are skipped, an empty result keeps the
-- built-in list.
function _M.set_signatures(list)
  if type(list) ~= "table" then return end
  local out = {}
  for _, s in ipairs(list) do
    if type(s) == "table" and type(s.id) == "string" and type(s.pattern) == "string" then
      local opts = (type(s.options) == "string" and s.options ~= "") and s.options or "jo"
      -- ngx.re.find returns nil, nil, err for a pattern PCRE rejects
      local ok, _, _, err = pcall(re_find, "", s.pattern, opts)
      if ok and not err then
        out[#out + 1] = { s.id, s.pattern, opts }
      else
        ngx.log(ngx.ERR, "corvus body: invalid signature ", s.id, " ignored")
      end
    end
  end
  if #out > 0 then SIGNATURES = out end
end

-- Exposed for tests.
_M._normalise = normalise
_M._parse_params = parse_params
_M.SIGNATURES = SIGNATURES
_M.MAX_ITEMS = MAX_ITEMS

return _M
