-- Client keys: IPv4 address, IPv6 /64 (automatic bans, per-client limits).
package.path = "gateway/lua/?.lua;gateway/test/?.lua;" .. package.path
local T = require("stub")
local eq = T.eq
local k = require("corvus.ipkey")

eq(k.client("192.0.2.7"), "192.0.2.7", "IPv4 as is")
eq(k.client("2001:db8:1:2:aaaa::1"), "2001:0db8:0001:0002", "IPv6 /64")
eq(k.client("2001:DB8:1:2:bbbb:cccc:dddd:eeee"), "2001:0db8:0001:0002", "same /64, other address")
eq(k.client("::ffff:192.0.2.1"), "::ffff:192.0.2.1", "unparsed form kept")
eq(k.client(nil), "", "no address")
eq(k.v6_prefix("2001:db8::a:b:c:d", 3), "2001:0db8:0000", "/48")

T.done("ipkey_test")
