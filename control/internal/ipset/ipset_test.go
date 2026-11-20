package ipset

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/netcfg"
)

func mustSet(t *testing.T, entries ...string) *Set {
	t.Helper()
	s, err := FromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ranges(s *Set) string {
	return strings.Join(s.Preview(1<<30), ",")
}

func TestMerge(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		// duplicates, overlap, containment
		{[]string{"10.0.0.1", "10.0.0.1", "10.0.0.0/24", "10.0.0.128/25"}, "10.0.0.0/24"},
		// adjacent networks merge, a gap does not
		{[]string{"10.0.0.0/25", "10.0.0.128/25", "10.0.1.0/24"}, "10.0.0.0/23"},
		{[]string{"1.2.3.5", "1.2.3.6", "1.2.3.8"}, "1.2.3.5-1.2.3.6,1.2.3.8"},
		{[]string{"1.2.3.6", "1.2.3.5", "1.2.3.4", "1.2.3.7"}, "1.2.3.4/30"},
		// /0 and the ends of the address space
		{[]string{"0.0.0.0/0", "1.2.3.4"}, "0.0.0.0/0"},
		{[]string{"255.255.255.255", "255.255.255.254", "0.0.0.0"}, "0.0.0.0,255.255.255.254/31"},
		{[]string{"::/0", "2001:db8::1"}, "::/0"},
		{[]string{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "::"}, "::,ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
		// IPv6 adjacency across the 64-bit halves
		{[]string{"2001:db8::/65", "2001:db8:0:0:8000::/65"}, "2001:db8::/64"},
		{[]string{"2001:db8:0:0:ffff:ffff:ffff:ffff", "2001:db8:0:1::"}, "2001:db8::ffff:ffff:ffff:ffff-2001:db8:0:1::"},
		// mixed families; IPv4-mapped networks become IPv4 (broad mapped ones are left out)
		{[]string{"2001:db8::/32", "::ffff:192.0.2.0/120", "192.0.2.0/24", "::ffff:0:0/96", "198.51.100.7"}, "192.0.2.0/24,198.51.100.7,2001:db8::/32"},
		// host bits are cleared
		{[]string{"192.0.2.77/24"}, "192.0.2.0/24"},
	}
	for _, c := range cases {
		if got := ranges(mustSet(t, c.in...)); got != c.want {
			t.Errorf("%v: got %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := FromEntries([]string{"1.2.3.4", "nope"}); err == nil {
		t.Error("invalid entry accepted")
	}
	for _, bad := range []string{"fe80::1%eth0", "fe80::%eth0/64", "1.2.3.4/33", "1.2.3", ""} {
		if _, ok, err := ParseEntry(bad); err == nil && ok {
			t.Errorf("ParseEntry(%q) accepted", bad)
		}
	}
}

// ParseEntry must normalise exactly like the validation of manually edited lists.
func TestParseEntryMatchesNormalizeCIDRs(t *testing.T) {
	for _, e := range []string{"1.2.3.4", "1.2.3.4/24", "::ffff:1.2.3.4", "::ffff:1.2.3.0/120", "::ffff:0:0/96",
		"::ffff:1.2.3.4/80", "2001:db8::1", "2001:db8::1/48", "::/0", "0.0.0.0/0", "10.0.0.0/8", "1.2.3.4/024", "  9.9.9.9  "} {
		want, err := netcfg.NormalizeCIDRs([]string{e})
		if err != nil {
			t.Fatal(e, err)
		}
		p, ok, err := ParseEntry(e)
		if err != nil {
			t.Errorf("%q: %v", e, err)
			continue
		}
		got := []string{}
		if ok {
			got = append(got, p.String())
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%q: ParseEntry %v, NormalizeCIDRs %v", e, got, want)
		}
	}
}

func TestContainsAndBoundaries(t *testing.T) {
	s := mustSet(t, "10.0.0.0/24", "10.0.2.0/24", "192.0.2.9", "2001:db8::/48", "2001:db8:2::5", "0.0.0.0", "255.255.255.255")
	for addr, want := range map[string]bool{
		"0.0.0.0": true, "0.0.0.1": false, "9.255.255.255": false, "10.0.0.0": true, "10.0.0.255": true,
		"10.0.1.0": false, "10.0.2.128": true, "10.0.3.0": false, "192.0.2.9": true, "192.0.2.8": false,
		"255.255.255.254": false, "255.255.255.255": true,
		"2001:db8::": true, "2001:db8:0:ffff:ffff:ffff:ffff:ffff": true, "2001:db8:1::": false,
		"2001:db8:2::5": true, "2001:db8:2::6": false, "::": false, "::ffff:10.0.0.1": false,
	} {
		got, _ := s.Contains(netip.MustParseAddr(addr))
		if got != want {
			t.Errorf("Contains(%s) = %v, want %v", addr, got, want)
		}
	}
	empty := &Set{}
	if ok, _ := empty.Contains(netip.MustParseAddr("1.2.3.4")); ok {
		t.Error("empty set matched")
	}
	if _, r := s.Contains(netip.MustParseAddr("10.0.2.1")); r != "10.0.2.0/24" {
		t.Errorf("range = %q", r)
	}
}

func TestRangeString(t *testing.T) {
	for _, c := range []struct {
		r    Range6
		want string
	}{
		{Range6{U128{1, 0}, U128{1, 5}}, "0:0:0:1::-::1:0:0:0:5"},
		{Range6{U128{0, 0}, U128{^uint64(0), ^uint64(0)}}, "::/0"},
		{Range6{U128{2, 0}, U128{3, ^uint64(0)}}, "0:0:0:2::/63"},
		{Range6{U128{1, 0}, U128{2, ^uint64(0)}}, "0:0:0:1::-::2:ffff:ffff:ffff:ffff"},
	} {
		if got := c.r.String(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.r, got, c.want)
		}
	}
	if got := (Range4{0, ^uint32(0)}).String(); got != "0.0.0.0/0" {
		t.Errorf("v4 /0: %q", got)
	}
	if got := (Range4{1, 2}).String(); got != "0.0.0.1-0.0.0.2" {
		t.Errorf("unaligned: %q", got)
	}
}

func randomSet(r *rand.Rand, n4, n6 int) *Set {
	var b Builder
	for i := 0; i < n4; i++ {
		var a [4]byte
		binary.BigEndian.PutUint32(a[:], r.Uint32())
		b.Add(netip.PrefixFrom(netip.AddrFrom4(a), 16+r.IntN(17)))
	}
	for i := 0; i < n6; i++ {
		var a [16]byte
		binary.BigEndian.PutUint64(a[:], r.Uint64())
		binary.BigEndian.PutUint64(a[8:], r.Uint64())
		b.Add(netip.PrefixFrom(netip.AddrFrom16(a), 32+r.IntN(97)))
	}
	return b.Set()
}

// The merged set answers exactly like a naive scan of the networks it was built from.
func TestMergeAgainstNaive(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var nets []netip.Prefix
	var b Builder
	for i := 0; i < 3000; i++ {
		var a [4]byte
		binary.BigEndian.PutUint32(a[:], 0x0a000000|r.Uint32()&0xffff)
		p := netip.PrefixFrom(netip.AddrFrom4(a), 20+r.IntN(13)).Masked()
		nets = append(nets, p)
		b.Add(p)
	}
	s := b.Set()
	for i := 0; i < 20000; i++ {
		var a [4]byte
		binary.BigEndian.PutUint32(a[:], 0x0a000000|r.Uint32()&0x1ffff)
		ip := netip.AddrFrom4(a)
		want := false
		for _, p := range nets {
			if p.Contains(ip) {
				want = true
				break
			}
		}
		if got, _ := s.Contains(ip); got != want {
			t.Fatalf("%s: got %v want %v", ip, got, want)
		}
	}
	for i := 1; i < len(s.V4); i++ {
		if s.V4[i].First <= s.V4[i-1].Last+1 {
			t.Fatalf("ranges %d and %d overlap or touch", i-1, i)
		}
	}
}

func TestRoundTripAndCorruption(t *testing.T) {
	s := randomSet(rand.New(rand.NewPCG(3, 4)), 5000, 700)
	data := s.Encode()
	if int64(len(data)) != (Header{N4: len(s.V4), N6: len(s.V6)}).Size() {
		t.Fatalf("size %d", len(data))
	}
	got, sum, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256(data)); sum != want {
		t.Fatalf("sum %s, want %s", sum, want)
	}
	if ranges(got) != ranges(s) {
		t.Fatal("round trip changed the set")
	}
	if h, _, err := Verify(bytes.NewReader(data)); err != nil || h.N4 != len(s.V4) || h.N6 != len(s.V6) {
		t.Fatalf("verify: %+v %v", h, err)
	}
	// empty set
	if e, _, err := Read(bytes.NewReader((&Set{}).Encode())); err != nil || e.Ranges() != 0 {
		t.Fatalf("empty: %v", err)
	}

	mod := func(f func(b []byte) []byte) []byte { return f(append([]byte(nil), data...)) }
	bad := map[string]struct {
		b    []byte
		want error
	}{
		"magic":     {mod(func(b []byte) []byte { b[0] = 'X'; return b }), ErrFormat},
		"version":   {mod(func(b []byte) []byte { b[8] = 2; return b }), ErrFormat},
		"short":     {data[:40], ErrFormat},
		"truncated": {data[:len(data)-1], ErrSize},
		"trailing":  {append(append([]byte(nil), data...), 0), ErrSize},
		"checksum":  {mod(func(b []byte) []byte { b[32] ^= 1; return b }), ErrChecksum},
		"count":     {mod(func(b []byte) []byte { b[16]--; return b }), ErrOrder}, // last v4 record read as v6 start → size/order
		"order": {mod(func(b []byte) []byte {
			copy(b[HeaderSize:], b[HeaderSize+8:HeaderSize+16]) // duplicate record 1 into 0
			return b
		}), ErrOrder},
	}
	for name, c := range bad {
		_, _, err := Read(bytes.NewReader(c.b))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if name != "count" && !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestDir(t *testing.T) {
	d := &Dir{Path: filepath.Join(t.TempDir(), "lists")}
	s := mustSet(t, "192.0.2.0/24", "198.51.100.1", "2001:db8::/32")
	info, err := d.Write(s)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidHash(info.Hash) || info.Ranges != 3 || !FileName.MatchString(filepath.Base(d.File(info.Hash))) {
		t.Fatalf("info %+v", info)
	}
	if h, err := d.Check(info.Hash); err != nil || h.N4 != 2 || h.N6 != 1 {
		t.Fatalf("check: %+v %v", h, err)
	}
	// writing again: same file, refreshed modification time
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(d.File(info.Hash), old, old)
	again, err := d.Write(s)
	if err != nil || again != info {
		t.Fatalf("rewrite: %+v %v", again, err)
	}
	if fi, _ := os.Stat(d.File(info.Hash)); time.Since(fi.ModTime()) > time.Minute {
		t.Fatal("modification time not refreshed")
	}
	if left, _ := filepath.Glob(filepath.Join(d.Path, ".tmp-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
	l, err := d.Load(info.Hash)
	if err != nil || ranges(l) != ranges(s) {
		t.Fatalf("load: %v", err)
	}

	f, err := d.Open(info.Hash)
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{"192.0.2.200": "192.0.2.0/24", "198.51.100.1": "198.51.100.1", "198.51.100.2": "",
		"1.1.1.1": "", "2001:db8:ffff::1": "2001:db8::/32", "2001:db9::": "", "::1": ""} {
		ok, r, err := f.Contains(netip.MustParseAddr(addr))
		if err != nil || ok != (want != "") || r != want {
			t.Errorf("file Contains(%s) = %v %q %v", addr, ok, r, err)
		}
	}
	if p, err := f.Preview(2); err != nil || strings.Join(p, ",") != "192.0.2.0/24,198.51.100.1" {
		t.Errorf("preview %v %v", p, err)
	}
	if p, _ := f.Preview(10); len(p) != 3 {
		t.Errorf("preview all %v", p)
	}
	f.Close()

	// a damaged file is detected (also after a successful check: size/mtime changed)
	b, _ := os.ReadFile(d.File(info.Hash))
	b[len(b)-1] ^= 0xff
	if err := os.WriteFile(d.File(info.Hash), b, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(d.File(info.Hash), old, old)
	if _, err := d.Check(info.Hash); err == nil {
		t.Fatal("damaged file passed the check")
	}
	if _, err := d.Load(info.Hash); err == nil {
		t.Fatal("damaged file loaded")
	}
	// ... and replaced by the next write of the same set
	if _, err := d.Write(s); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Check(info.Hash); err != nil {
		t.Fatalf("not repaired: %v", err)
	}

	missing := strings.Repeat("ab", 32)
	if _, err := d.Check(missing); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := d.Open(missing); !errors.Is(err, ErrMissing) {
		t.Fatalf("open missing: %v", err)
	}
	if _, err := d.Load(missing); !errors.Is(err, ErrMissing) {
		t.Fatalf("load missing: %v", err)
	}
	for _, h := range []string{"../x", "AB" + strings.Repeat("0", 62)} {
		if _, err := d.Check(h); err == nil {
			t.Errorf("hash %q accepted", h)
		}
		if _, err := d.Open(h); err == nil {
			t.Errorf("open %q accepted", h)
		}
		if _, err := d.Load(h); err == nil {
			t.Errorf("load %q accepted", h)
		}
	}
	// a file whose header does not match its size cannot be opened
	if err := os.WriteFile(d.File(missing), (&Set{}).Encode()[:HeaderSize-1], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Open(missing); err == nil {
		t.Fatal("short file opened")
	}
}

// Streaming a builder (several chunks per family) gives the same file as writing the set.
func TestWriteBuilderMatchesWrite(t *testing.T) {
	d := &Dir{Path: t.TempDir()}
	fill := func() *Builder {
		r := rand.New(rand.NewPCG(11, 12))
		b := &Builder{}
		for i := 0; i < 3*chunk4+123; i++ {
			var a [4]byte
			binary.BigEndian.PutUint32(a[:], r.Uint32()&0x00ffffff) // dense: many merges and duplicates
			b.Add(netip.PrefixFrom(netip.AddrFrom4(a), 30+r.IntN(3)))
		}
		for i := 0; i < 2*chunk6+7; i++ {
			var a [16]byte
			a[0], a[1], a[15] = 0x20, 0x01, byte(r.IntN(256))
			binary.BigEndian.PutUint16(a[6:], uint16(r.IntN(4096)))
			b.Add(netip.PrefixFrom(netip.AddrFrom16(a), 120+r.IntN(9)))
		}
		return b
	}
	b := fill()
	if b.Len() != 3*chunk4+123+2*chunk6+7 {
		t.Fatalf("len %d", b.Len())
	}
	streamed, err := d.WriteBuilder(b)
	if err != nil {
		t.Fatal(err)
	}
	set := fill().Set()
	if set.Networks != streamed.Networks || set.Networks >= 3*chunk4+123+2*chunk6+7 {
		t.Fatalf("networks %d vs %d", set.Networks, streamed.Networks)
	}
	written, err := d.Write(set)
	if err != nil {
		t.Fatal(err)
	}
	if written.Hash != streamed.Hash || written.Ranges != set.Ranges() {
		t.Fatalf("streamed %+v, written %+v", streamed, written)
	}
	if _, err := d.Check(written.Hash); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(set.Encode(), mustRead(t, d.File(written.Hash))) {
		t.Fatal("Encode and Write differ")
	}
	for i := 1; i < len(set.V4); i++ {
		if set.V4[i].First <= set.V4[i-1].Last+1 {
			t.Fatal("v4 ranges not merged")
		}
	}
	if b.Len() != 0 {
		t.Fatal("builder not emptied")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseEntryBytes(t *testing.T) {
	for _, s := range []string{"1.2.3.4", "1.2.3.4/24", "0.0.0.0/0", "255.255.255.255/32", "1.2.3.4/024", "01.2.3.4", "1.2.3.256",
		"1.2.3.4/33", "1.2.3", "1.2.3.4.5", "1.2.3.4/", "::1", "2001:db8::/32", "::ffff:1.2.3.4", "1.2.3.4x", "1..3.4", "1.2.3.4/3x", "1.2.3.1000"} {
		p1, ok1, err1 := ParseEntry(s)
		p2, ok2, err2 := ParseEntryBytes([]byte(s))
		if p1 != p2 || ok1 != ok2 || (err1 == nil) != (err2 == nil) {
			t.Errorf("%q: ParseEntry %v %v %v, ParseEntryBytes %v %v %v", s, p1, ok1, err1, p2, ok2, err2)
		}
	}
}

// goldenFile is shared with the gateway's Lua tests (gateway/test/iplist_test.lua): both
// sides must agree on the format. Regenerate with CORVUS_UPDATE_GOLDEN=1.
const goldenFile = "../../../gateway/test/testdata/iplist.bin"

func goldenSet(t *testing.T) *Set {
	return mustSet(t, "0.0.0.0", "10.0.0.0/8", "11.0.0.0/8", "192.0.2.1", "192.0.2.3", "198.51.100.0/24", "255.255.255.255",
		"::", "2001:db8::/48", "2001:db8:0:0:ffff::/80", "2001:db8:1::5", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
}

func TestGoldenFile(t *testing.T) {
	data := goldenSet(t).Encode()
	if os.Getenv("CORVUS_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(goldenFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	have, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(have, data) {
		t.Fatalf("%s is out of date (CORVUS_UPDATE_GOLDEN=1)", goldenFile)
	}
	t.Logf("golden sha256 %x", sha256.Sum256(data))
}

func BenchmarkBuild(b *testing.B) {
	r := rand.New(rand.NewPCG(5, 6))
	addrs := make([]netip.Prefix, 1_000_000)
	for i := range addrs {
		var a [4]byte
		binary.BigEndian.PutUint32(a[:], r.Uint32())
		addrs[i] = netip.PrefixFrom(netip.AddrFrom4(a), 32)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var bl Builder
		for _, p := range addrs {
			bl.Add(p)
		}
		_ = bl.Set()
	}
}

func BenchmarkContains(b *testing.B) {
	s := randomSet(rand.New(rand.NewPCG(7, 8)), 1_000_000, 0)
	r := rand.New(rand.NewPCG(9, 10))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var a [4]byte
		binary.BigEndian.PutUint32(a[:], r.Uint32())
		s.Contains(netip.AddrFrom4(a))
	}
}
