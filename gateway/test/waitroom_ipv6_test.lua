-- IPv6 network prefixes used by the access queue (token binding /48, per-client limits /64).
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
-- only the pure helpers are tested: the OpenResty-only modules are not used
package.loaded["resty.redis"] = {}
package.loaded["cjson.safe"] = {}
package.loaded["resty.string"] = {}
local eq = T.eq
local wr = require("corvus.waitroom")
local p = wr._v6_prefix

eq(p("2001:db8::a:b:c:d", 3), "2001:0db8:0000", "compressed in the middle")
eq(p("2001:db8:0:0:a::", 3), "2001:0db8:0000", "same /48, other spelling")
eq(p("2001:DB8:0:0:a::", 4), "2001:0db8:0000:0000", "upper case, /64")
eq(p("2001:db8:1:2:3:4:5:6", 4), "2001:0db8:0001:0002", "full form")
eq(p("::1", 3), "0000:0000:0000", "loopback")
eq(p("fe80::", 4), "fe80:0000:0000:0000", "trailing ::")
eq(p("::ffff:192.0.2.1", 3), nil, "embedded IPv4 is not handled")
eq(p("1:2:3:4:5:6:7", 3), nil, "too few groups")
eq(p("1::2::3", 3), nil, "two ::")

T.done("waitroom_ipv6_test")
