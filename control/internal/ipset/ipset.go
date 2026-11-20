// Package ipset keeps IP lists as sorted, non-overlapping, inclusive address ranges and
// stores them in a compact binary file that the gateway maps read-only into memory
// (gateway/lua/corvus/iplist.lua), so a list of millions of entries costs its file size
// once (shared page cache), not a Lua table per nginx worker.
//
// File format (version 1), all integers little-endian:
//
//	offset  size  field
//	0       8     magic "CVIPSET\x00"
//	8       4     format version (1)
//	12      4     reserved (0)
//	16      8     n4: number of IPv4 ranges
//	24      8     n6: number of IPv6 ranges
//	32      32    SHA-256 of the payload (everything after the header)
//	64      8·n4  IPv4 ranges: uint32 first, uint32 last
//	…       32·n6 IPv6 ranges: uint64 first_hi, first_lo, last_hi, last_lo
//
// An IPv6 address is two 64-bit halves: hi is bytes 0-7 of the address read as a
// big-endian number, lo bytes 8-15. Ranges are inclusive, sorted by first address and
// neither overlap nor touch (adjacent ranges are merged). The file is named after the
// SHA-256 of its whole content (<sha256>.bin, see Dir).
package ipset

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/bits"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// Format constants.
const (
	Magic      = "CVIPSET\x00"
	Version    = 1
	HeaderSize = 64
	size4      = 8  // bytes per IPv4 range
	size6      = 32 // bytes per IPv6 range
)

// U128 is an IPv6 address as a number.
type U128 struct{ Hi, Lo uint64 }

func (a U128) cmp(b U128) int {
	if c := cmp.Compare(a.Hi, b.Hi); c != 0 {
		return c
	}
	return cmp.Compare(a.Lo, b.Lo)
}

func (a U128) inc() (U128, bool) {
	lo, carry := bits.Add64(a.Lo, 1, 0)
	hi, over := bits.Add64(a.Hi, 0, carry)
	return U128{hi, lo}, over != 0
}

func u128(a netip.Addr) U128 {
	b := a.As16()
	return U128{binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])}
}

func (a U128) addr() netip.Addr {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], a.Hi)
	binary.BigEndian.PutUint64(b[8:], a.Lo)
	return netip.AddrFrom16(b)
}

// Range4 and Range6 are inclusive address ranges.
type Range4 struct{ First, Last uint32 }
type Range6 struct{ First, Last U128 }

// Set is a merged list: sorted, non-overlapping, non-adjacent ranges per family.
type Set struct {
	V4 []Range4
	V6 []Range6
	// Networks is the number of distinct networks the set was built from (not stored
	// in the file).
	Networks int
}

// Ranges is the number of ranges of both families.
func (s *Set) Ranges() int { return len(s.V4) + len(s.V6) }

// ------------------------------------------------------------------ parsing

// ParseEntry parses one list entry (an IP address or a CIDR) with the rules of
// netcfg.NormalizeCIDRs: a bare address is a /32 or /128, host bits are cleared, and an
// IPv4-mapped IPv6 network (::ffff:a.b.c.d/p) of /104 or narrower is that IPv4 network;
// broader mapped networks never match an address the gateway sees and are left out
// (ok = false, no error).
func ParseEntry(s string) (p netip.Prefix, ok bool, err error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err = netip.ParsePrefix(s)
		if err != nil {
			// net.ParseCIDR accepts a few spellings netip does not (e.g. "/024"):
			// stay compatible with the lists validated by netcfg.
			_, n, nerr := net.ParseCIDR(s)
			if nerr != nil {
				return p, false, fmt.Errorf("invalid CIDR %q", s)
			}
			a, _ := netip.AddrFromSlice(n.IP)
			ones, _ := n.Mask.Size()
			p = netip.PrefixFrom(a, ones)
		}
	} else {
		a, err := netip.ParseAddr(s)
		if err != nil || a.Zone() != "" {
			return p, false, fmt.Errorf("invalid IP %q", s)
		}
		p = netip.PrefixFrom(a, a.BitLen())
	}
	if p.Addr().Zone() != "" {
		return p, false, fmt.Errorf("invalid CIDR %q", s)
	}
	p = p.Masked()
	if a := p.Addr(); a.Is4In6() {
		if p.Bits() < 104 {
			return p, false, nil
		}
		p = netip.PrefixFrom(a.Unmap(), p.Bits()-96)
	}
	return p, true, nil
}

// ParseEntryBytes is ParseEntry for a byte slice (one line of a feed). The common case,
// an IPv4 address or CIDR in canonical form, is parsed without allocating.
func ParseEntryBytes(b []byte) (netip.Prefix, bool, error) {
	if p, ok := parse4(b); ok {
		return p, true, nil
	}
	return ParseEntry(string(b))
}

// parse4 parses "a.b.c.d" or "a.b.c.d/n" (decimal octets without leading zeros, n 0-32
// without leading zeros); anything else is left to ParseEntry.
func parse4(b []byte) (netip.Prefix, bool) {
	var a [4]byte
	i := 0
	for k := 0; k < 4; k++ {
		if k > 0 {
			if i >= len(b) || b[i] != '.' {
				return netip.Prefix{}, false
			}
			i++
		}
		v, n := decimal(b[i:], 255)
		if n == 0 {
			return netip.Prefix{}, false
		}
		a[k], i = byte(v), i+n
	}
	bits := 32
	if i < len(b) {
		if b[i] != '/' {
			return netip.Prefix{}, false
		}
		v, n := decimal(b[i+1:], 32)
		if n == 0 || i+1+n != len(b) {
			return netip.Prefix{}, false
		}
		bits = v
	}
	return netip.PrefixFrom(netip.AddrFrom4(a), bits).Masked(), true
}

// decimal reads a number <= max without leading zeros at the start of b; n is the
// number of digits read (0: none or invalid).
func decimal(b []byte, max int) (v, n int) {
	for n < len(b) && n < 3 && b[n] >= '0' && b[n] <= '9' {
		v = v*10 + int(b[n]-'0')
		n++
	}
	if n == 0 || (n > 1 && b[0] == '0') || v > max || (n < len(b) && b[n] >= '0' && b[n] <= '9') {
		return 0, 0
	}
	return v, n
}

// FromEntries builds a set from list entries (see ParseEntry); an invalid entry is an error.
func FromEntries(entries []string) (*Set, error) {
	var b Builder
	for _, e := range entries {
		if strings.TrimSpace(e) == "" {
			continue
		}
		p, ok, err := ParseEntry(e)
		if err != nil {
			return nil, err
		}
		if ok {
			b.Add(p)
		}
	}
	return b.Set(), nil
}

// ------------------------------------------------------------------ lookups

// Contains reports whether addr is in the set (an IPv4-mapped IPv6 address is looked up
// as IPv6, like the gateway does with the client address it sees).
func (s *Set) Contains(addr netip.Addr) (bool, string) {
	if addr.Is4() {
		x := binary.BigEndian.Uint32(addr.AsSlice())
		i, found := slices.BinarySearchFunc(s.V4, x, func(r Range4, x uint32) int { return cmp.Compare(r.First, x) })
		if !found {
			i--
		}
		if i >= 0 && x <= s.V4[i].Last {
			return true, s.V4[i].String()
		}
		return false, ""
	}
	x := u128(addr)
	i, found := slices.BinarySearchFunc(s.V6, x, func(r Range6, x U128) int { return r.First.cmp(x) })
	if !found {
		i--
	}
	if i >= 0 && x.cmp(s.V6[i].Last) <= 0 {
		return true, s.V6[i].String()
	}
	return false, ""
}

func addr4(x uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], x)
	return netip.AddrFrom4(b)
}

// String formats a range as an address, a CIDR when it is exactly one, or "first-last".
func (r Range4) String() string {
	if r.First == r.Last {
		return addr4(r.First).String()
	}
	span := r.Last - r.First
	if span&(span+1) == 0 && r.First&span == 0 { // span+1 is a power of two, aligned
		return netip.PrefixFrom(addr4(r.First), 32-bits.OnesCount32(span)).String()
	}
	return addr4(r.First).String() + "-" + addr4(r.Last).String()
}

func (r Range6) String() string {
	if r.First == r.Last {
		return r.First.addr().String()
	}
	span := U128{r.Last.Hi - r.First.Hi, r.Last.Lo - r.First.Lo}
	if r.Last.Lo < r.First.Lo {
		span.Hi--
	}
	ones := bits.OnesCount64(span.Hi) + bits.OnesCount64(span.Lo)
	aligned := span.Hi&r.First.Hi == 0 && span.Lo&r.First.Lo == 0
	// span must be 2^k - 1: all ones in the low k bits
	lowOnes := (span.Hi == 0 && span.Lo&(span.Lo+1) == 0) || (span.Lo == ^uint64(0) && span.Hi&(span.Hi+1) == 0)
	if aligned && lowOnes {
		return netip.PrefixFrom(r.First.addr(), 128-ones).String()
	}
	return r.First.addr().String() + "-" + r.Last.addr().String()
}

// Preview returns up to n ranges (IPv4 first), formatted with String.
func (s *Set) Preview(n int) []string {
	out := []string{}
	for _, r := range s.V4 {
		if len(out) >= n {
			return out
		}
		out = append(out, r.String())
	}
	for _, r := range s.V6 {
		if len(out) >= n {
			return out
		}
		out = append(out, r.String())
	}
	return out
}

// ------------------------------------------------------------------ encoding

// Header is the decoded file header.
type Header struct {
	N4, N6  int
	Payload [32]byte // SHA-256 of the payload
}

// Size is the file size the header implies.
func (h Header) Size() int64 { return HeaderSize + int64(h.N4)*size4 + int64(h.N6)*size6 }

// writePayload writes the ranges in file order, in chunks.
func (s *Set) writePayload(w io.Writer) error {
	buf := make([]byte, 0, 64<<10)
	flush := func(force bool) error {
		if len(buf) == 0 || (!force && len(buf)+size6 <= cap(buf)) {
			return nil
		}
		_, err := w.Write(buf)
		buf = buf[:0]
		return err
	}
	for _, r := range s.V4 {
		buf = binary.LittleEndian.AppendUint32(buf, r.First)
		buf = binary.LittleEndian.AppendUint32(buf, r.Last)
		if err := flush(false); err != nil {
			return err
		}
	}
	for _, r := range s.V6 {
		buf = binary.LittleEndian.AppendUint64(buf, r.First.Hi)
		buf = binary.LittleEndian.AppendUint64(buf, r.First.Lo)
		buf = binary.LittleEndian.AppendUint64(buf, r.Last.Hi)
		buf = binary.LittleEndian.AppendUint64(buf, r.Last.Lo)
		if err := flush(false); err != nil {
			return err
		}
	}
	return flush(true)
}

func (s *Set) header() []byte {
	h := sha256.New()
	_ = s.writePayload(h)
	return header(len(s.V4), len(s.V6), h.Sum(nil))
}

func header(n4, n6 int, payloadSum []byte) []byte {
	b := make([]byte, HeaderSize)
	copy(b, Magic)
	binary.LittleEndian.PutUint32(b[8:], Version)
	binary.LittleEndian.PutUint64(b[16:], uint64(n4))
	binary.LittleEndian.PutUint64(b[24:], uint64(n6))
	copy(b[32:], payloadSum)
	return b
}

// WriteTo writes the file and returns its size; the SHA-256 of what was written is the
// file's name (see Dir).
func (s *Set) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	if _, err := cw.Write(s.header()); err != nil {
		return cw.n, err
	}
	err := s.writePayload(cw)
	return cw.n, err
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Encode returns the file content.
func (s *Set) Encode() []byte {
	var b strings.Builder
	_, _ = s.WriteTo(&b)
	return []byte(b.String())
}

// Errors of a damaged file.
var (
	ErrFormat   = errors.New("not an IP list file or unsupported version")
	ErrSize     = errors.New("file size does not match its header")
	ErrChecksum = errors.New("checksum mismatch")
	ErrOrder    = errors.New("ranges are not sorted and disjoint")
)

// ParseHeader decodes and checks the header (not the payload).
func ParseHeader(b []byte) (Header, error) {
	var h Header
	if len(b) < HeaderSize || string(b[:8]) != Magic || binary.LittleEndian.Uint32(b[8:]) != Version {
		return h, ErrFormat
	}
	n4, n6 := binary.LittleEndian.Uint64(b[16:]), binary.LittleEndian.Uint64(b[24:])
	if n4 > 1<<40 || n6 > 1<<40 {
		return h, ErrSize
	}
	h.N4, h.N6 = int(n4), int(n6)
	copy(h.Payload[:], b[32:64])
	return h, nil
}

// Read decodes and fully checks a file (header, size, payload checksum, order). It
// returns the SHA-256 of the whole file as well.
func Read(r io.Reader) (*Set, string, error) {
	_, s, sum, err := read(r, true)
	return s, sum, err
}

// Verify checks a file like Read without keeping the ranges.
func Verify(r io.Reader) (Header, string, error) {
	h, _, sum, err := read(r, false)
	return h, sum, err
}

func read(r io.Reader, keep bool) (Header, *Set, string, error) {
	var h Header
	file := sha256.New()
	r = io.TeeReader(r, file)
	hb := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, hb); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return h, nil, "", ErrFormat
		}
		return h, nil, "", err
	}
	h, err := ParseHeader(hb)
	if err != nil {
		return h, nil, "", err
	}
	payload := sha256.New()
	pr := &chunkReader{r: io.TeeReader(r, payload)}
	s := &Set{}
	if keep {
		s.V4 = make([]Range4, 0, min(h.N4, 1<<20))
		s.V6 = make([]Range6, 0, min(h.N6, 1<<16))
	}
	var prev4 Range4
	for i := 0; i < h.N4; i++ {
		b, err := pr.next(size4)
		if err != nil {
			return h, nil, "", err
		}
		x := Range4{binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:])}
		if x.First > x.Last || (i > 0 && x.First <= prev4.Last) {
			return h, nil, "", ErrOrder
		}
		prev4 = x
		if keep {
			s.V4 = append(s.V4, x)
		}
	}
	var prev6 Range6
	for i := 0; i < h.N6; i++ {
		b, err := pr.next(size6)
		if err != nil {
			return h, nil, "", err
		}
		x := Range6{U128{binary.LittleEndian.Uint64(b), binary.LittleEndian.Uint64(b[8:])},
			U128{binary.LittleEndian.Uint64(b[16:]), binary.LittleEndian.Uint64(b[24:])}}
		if x.First.cmp(x.Last) > 0 || (i > 0 && x.First.cmp(prev6.Last) <= 0) {
			return h, nil, "", ErrOrder
		}
		prev6 = x
		if keep {
			s.V6 = append(s.V6, x)
		}
	}
	if n, _ := io.Copy(io.Discard, r); n > 0 || len(pr.buf) > pr.off {
		return h, nil, "", ErrSize
	}
	if [32]byte(payload.Sum(nil)) != h.Payload {
		return h, nil, "", ErrChecksum
	}
	return h, s, hexSum(file), nil
}

func hexSum(h hash.Hash) string { return fmt.Sprintf("%x", h.Sum(nil)) }

// chunkReader reads fixed-size records through a buffer.
type chunkReader struct {
	r   io.Reader
	buf []byte
	off int
}

func (c *chunkReader) next(n int) ([]byte, error) {
	if c.buf == nil {
		c.buf = make([]byte, 0, 64<<10)
	}
	if len(c.buf)-c.off < n {
		rest := copy(c.buf[:cap(c.buf)], c.buf[c.off:])
		m, err := io.ReadAtLeast(c.r, c.buf[rest:cap(c.buf)], n-rest)
		c.buf, c.off = c.buf[:rest+m], 0
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
				return nil, ErrSize
			}
			return nil, err
		}
	}
	b := c.buf[c.off : c.off+n]
	c.off += n
	return b, nil
}
