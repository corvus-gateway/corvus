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
-- Bodies above the host limit are not read (logged as "skip"); compressed request bodies
-- (Content-Encoding) cannot be inspected and are reported as "encoded".

local cjson_new = require("cjson.safe").new
local find, lower, sub, byte, gsub = string.find, string.lower, string.sub, string.byte, string.gsub
local re_find = ngx.re.find
local unescape = ngx.unescape_uri
local concat, insert = table.concat, table.insert

local _M = {}

local json = cjson_new()
json.decode_max_depth(64)

local MAX_ITEMS = 20000          -- normalised items per request
local MAX_PARTS = 1000           -- multipart parts before "anomaly"
local MAX_BRACKETS = 32          -- PHP array nesting in a field name before "anomaly"

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

-- Returns the raw body (possibly ""), or nil + reason ("too_large", "unreadable").
local function read_raw(max)
  local cl = tonumber(ngx.var.content_length)
  if cl == 0 then return "" end
  if not cl and not ngx.var.http_transfer_encoding then return "" end
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

local function add(acc, v)
  if acc.n >= MAX_ITEMS then return end
  acc.n = acc.n + 1
  acc[acc.n] = v
end

local flatten -- forward

local function looks_json(s)
  local b = byte(s, (find(s, "%S")) or 1)
  return b == 123 or b == 91 -- { or [
end

local function add_value(acc, v, depth)
  add(acc, v)
  if depth < 3 and #v > 1 and looks_json(v) then
    local ok, obj = pcall(json.decode, v)
    if ok and type(obj) == "table" then flatten(acc, obj, depth + 1) end
  end
end

flatten = function(acc, obj, depth)
  for k, v in pairs(obj) do
    if acc.n >= MAX_ITEMS then return end
    if type(k) == "string" then add(acc, k) end
    local t = type(v)
    if t == "string" then add_value(acc, v, depth)
    elseif t == "table" then flatten(acc, v, depth)
    end
  end
end

local function decode_form(acc, s)
  for pair in s:gmatch("[^&]+") do
    local k, v = pair:match("^([^=]*)=?(.*)$")
    for _, x in ipairs({ k, v }) do
      if x and x ~= "" then
        local d = unescape((gsub(x, "+", " ")))
        if find(d, "%", 1, true) then d = unescape(d) end
        add_value(acc, d, 1)
      end
    end
    if acc.n >= MAX_ITEMS then return end
  end
end

-- Multipart: returns false when the structure itself is anomalous.
local function decode_multipart(acc, s, ct, meta)
  -- the boundary is case-sensitive: read it from the original header value
  local boundary = ct:match('[Bb][Oo][Uu][Nn][Dd][Aa][Rr][Yy]="([^"]+)"') or ct:match("[Bb][Oo][Uu][Nn][Dd][Aa][Rr][Yy]=([^;%s]+)")
  if not boundary then add(acc, s) return true end
  local delim = "--" .. boundary
  local pos, parts = 1, 0
  while true do
    local a, b = find(s, delim, pos, true)
    if not a then break end
    local nexta = find(s, delim, b + 1, true)
    if not nexta then break end
    local part = sub(s, b + 1, nexta - 1)
    parts = parts + 1
    if parts > MAX_PARTS then meta.anomaly = true return false end
    local hdr_end, body_start = find(part, "\r\n\r\n", 1, true)
    if not hdr_end then hdr_end, body_start = find(part, "\n\n", 1, true) end
    if hdr_end then
      local hdr = lower(sub(part, 1, hdr_end))
      -- parameters start after ";" or whitespace: "xfilename=" or the "name=" inside
      -- "filename=" must not be taken for them
      local name = hdr:match('[;%s]name="([^"]*)"')
      local filename = hdr:match('[;%s]filename="([^"]*)"')
      if name then
        add(acc, name)
        local _, depth = gsub(name, "%[", "")
        if depth > MAX_BRACKETS then meta.anomaly = true end
      end
      if filename then
        add(acc, filename)                       -- file contents are not inspected
      else
        local v = sub(part, body_start + 1)
        v = gsub(v, "\r?\n$", "")
        add_value(acc, v, 1)
      end
    end
    pos = nexta
    if acc.n >= MAX_ITEMS then break end
  end
  meta.parts = parts
  return true
end

local function normalise(raw, ctype)
  local acc, meta = { n = 0 }, {}
  ctype = ctype or ""
  local ct = lower(ctype)
  if find(ct, "multipart/form-data", 1, true) then
    decode_multipart(acc, raw, ctype, meta)
  elseif find(ct, "json", 1, true) then
    local ok, obj = pcall(json.decode, raw)
    if ok and type(obj) == "table" then flatten(acc, obj, 1)
    elseif ok and type(obj) == "string" then add_value(acc, obj, 1)
    else add(acc, raw) end
  elseif find(ct, "x-www-form-urlencoded", 1, true) then
    decode_form(acc, raw)
  else
    add_value(acc, raw, 1)
  end
  acc.n = nil
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
  local cl = tonumber(ngx.var.http_content_length or "")
  local has_body = (cl ~= nil and cl > 0) or (ngx.var.http_transfer_encoding or "") ~= ""
  if has_body and enc and enc ~= "" and lower(enc) ~= "identity" then
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

-- inspect(max): returns a signature id when the body matches, "skip:<reason>" when the
-- body could not be inspected, or nil.
function _M.inspect(max)
  local text, err, meta = _M.text(max)
  if not text then
    if err == "encoded" then return "encoded" end
    return "skip:" .. tostring(err)
  end
  if meta and meta.anomaly then return "multipart-anomaly" end
  if text == "" then return nil end
  for _, s in ipairs(SIGNATURES) do
    local from = re_find(text, s[2], s[3])
    if from then return s[1] end
  end
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
      local ok, err = pcall(re_find, "", s.pattern, opts)
      if ok and err ~= false then
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
_M.SIGNATURES = SIGNATURES

return _M
