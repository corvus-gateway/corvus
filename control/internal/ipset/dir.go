package ipset

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Dir is the content-addressed store of list files: <Path>/<sha256>.bin, where sha256
// is the hash of the whole file. Files are written once (temporary file, fsync, rename)
// and never modified; corvus-control removes the ones nothing refers to any more
// (deploy.Deployer prune). Several configuration versions and lists share a file.
type Dir struct {
	Path string

	mu sync.Mutex
	ok map[string]stamp // files verified since their last change
}

type stamp struct {
	size int64
	mod  time.Time
	h    Header
}

// Info describes a stored file.
type Info struct {
	Hash     string `json:"hash"`
	Size     int64  `json:"size"`
	Ranges   int    `json:"ranges"`
	Networks int    `json:"networks"` // distinct networks merged into the ranges
}

var hashName = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidHash reports whether h has the form of a file hash.
func ValidHash(h string) bool { return hashName.MatchString(h) }

// FileName matches the names of list files.
var FileName = regexp.MustCompile(`^[0-9a-f]{64}\.bin$`)

// File is the path of the file with hash h.
func (d *Dir) File(h string) string { return filepath.Join(d.Path, h+".bin") }

// Write stores s and returns its hash. Writing a set that is already stored only
// refreshes the file's modification time (see Touch).
func (d *Dir) Write(s *Set) (Info, error) {
	return d.write(func(w4 func(Range4) error, w6 func(Range6) error) (int, error) {
		for _, r := range s.V4 {
			if err := w4(r); err != nil {
				return 0, err
			}
		}
		for _, r := range s.V6 {
			if err := w6(r); err != nil {
				return 0, err
			}
		}
		return s.Networks, nil
	})
}

// WriteBuilder stores the merged ranges of b without building the set in memory: the
// ranges are written as the builder produces them (see Builder). b is emptied.
func (d *Dir) WriteBuilder(b *Builder) (Info, error) {
	return d.write(func(w4 func(Range4) error, w6 func(Range6) error) (int, error) {
		var err error
		n := b.ranges(func(r Range4) error {
			err = w4(r)
			return err
		}, func(r Range6) error {
			err = w6(r)
			return err
		})
		return n, err
	})
}

// write writes a file from ranges produced in order (IPv4 first): the payload is
// streamed after a placeholder header, the header (counts, payload checksum) is filled
// in at the end, and the file is renamed to the SHA-256 of its content.
func (d *Dir) write(produce func(w4 func(Range4) error, w6 func(Range6) error) (int, error)) (Info, error) {
	if err := os.MkdirAll(d.Path, 0o755); err != nil {
		return Info{}, err
	}
	tmp, err := os.CreateTemp(d.Path, ".tmp-*")
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	defer tmp.Close()
	if _, err := tmp.Write(make([]byte, HeaderSize)); err != nil {
		return Info{}, err
	}
	payload := sha256.New()
	bw := bufio.NewWriterSize(io.MultiWriter(tmp, payload), 256<<10)
	var n4, n6 int
	var rec [size6]byte
	networks, err := produce(func(r Range4) error {
		if n6 > 0 {
			return ErrOrder
		}
		n4++
		binary.LittleEndian.PutUint32(rec[:], r.First)
		binary.LittleEndian.PutUint32(rec[4:], r.Last)
		_, err := bw.Write(rec[:size4])
		return err
	}, func(r Range6) error {
		n6++
		binary.LittleEndian.PutUint64(rec[:], r.First.Hi)
		binary.LittleEndian.PutUint64(rec[8:], r.First.Lo)
		binary.LittleEndian.PutUint64(rec[16:], r.Last.Hi)
		binary.LittleEndian.PutUint64(rec[24:], r.Last.Lo)
		_, err := bw.Write(rec[:])
		return err
	})
	if err == nil {
		err = bw.Flush()
	}
	if err != nil {
		return Info{}, err
	}
	if _, err := tmp.WriteAt(header(n4, n6, payload.Sum(nil)), 0); err != nil {
		return Info{}, err
	}
	// the name: SHA-256 of the whole file (read back from the page cache)
	sum := sha256.New()
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	size, err := io.Copy(sum, bufio.NewReaderSize(tmp, 256<<10))
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Info{}, err
	}
	info := Info{Hash: hexSum(sum), Size: size, Ranges: n4 + n6, Networks: networks}
	if _, err := d.Check(info.Hash); err == nil { // already stored (and intact)
		d.Touch(info.Hash)
		return info, nil
	}
	if err := os.Rename(tmp.Name(), d.File(info.Hash)); err != nil {
		return Info{}, err
	}
	syncDir(d.Path)
	return info, nil
}

func syncDir(p string) {
	if f, err := os.Open(p); err == nil {
		_ = f.Sync()
		f.Close()
	}
}

// Touch sets the modification time of a stored file to now, so a cleanup that runs
// concurrently keeps it (it spares recently changed files).
func (d *Dir) Touch(h string) {
	now := time.Now()
	if os.Chtimes(d.File(h), now, now) == nil {
		d.mu.Lock()
		if st, ok := d.ok[h]; ok {
			st.mod = modTime(d.File(h), st.mod)
			d.ok[h] = st
		}
		d.mu.Unlock()
	}
}

func modTime(p string, def time.Time) time.Time {
	if fi, err := os.Stat(p); err == nil {
		return fi.ModTime()
	}
	return def
}

// ErrMissing: no file with that hash.
var ErrMissing = errors.New("list file missing")

// Check verifies the stored file with hash h completely (header, size, checksums, range
// order) and returns its header. A successful check is remembered until the file's size
// or modification time changes.
func (d *Dir) Check(h string) (Header, error) {
	if !ValidHash(h) {
		return Header{}, fmt.Errorf("invalid list file hash %q", h)
	}
	p := d.File(h)
	fi, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Header{}, ErrMissing
		}
		return Header{}, err
	}
	d.mu.Lock()
	st, ok := d.ok[h]
	d.mu.Unlock()
	if ok && st.size == fi.Size() && st.mod.Equal(fi.ModTime()) {
		return st.h, nil
	}
	f, err := os.Open(p)
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	hd, sum, err := Verify(bufio.NewReaderSize(f, 256<<10))
	if err != nil {
		return Header{}, err
	}
	if sum != h {
		return Header{}, ErrChecksum
	}
	d.mu.Lock()
	if d.ok == nil {
		d.ok = map[string]stamp{}
	}
	d.ok[h] = stamp{size: fi.Size(), mod: fi.ModTime(), h: hd}
	d.mu.Unlock()
	return hd, nil
}

// Load reads and verifies the whole set with hash h.
func (d *Dir) Load(h string) (*Set, error) {
	if !ValidHash(h) {
		return nil, fmt.Errorf("invalid list file hash %q", h)
	}
	f, err := os.Open(d.File(h))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMissing
		}
		return nil, err
	}
	defer f.Close()
	s, sum, err := Read(bufio.NewReaderSize(f, 256<<10))
	if err != nil {
		return nil, err
	}
	if sum != h {
		return nil, ErrChecksum
	}
	return s, nil
}

// ------------------------------------------------------------------ reading in place

// File is a stored list opened for lookups without loading it: a binary search reads
// a few records (for the console's "check an IP" and preview).
type File struct {
	f *os.File
	h Header
}

// Open opens the file with hash h after checking its header and size (not its
// checksum: lookups through File are informative; the gateway and the configuration
// renderer verify files completely).
func (d *Dir) Open(h string) (*File, error) {
	if !ValidHash(h) {
		return nil, fmt.Errorf("invalid list file hash %q", h)
	}
	f, err := os.Open(d.File(h))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMissing
		}
		return nil, err
	}
	hb := make([]byte, HeaderSize)
	fi, err := f.Stat()
	if err == nil {
		_, err = io.ReadFull(f, hb)
	}
	var hd Header
	if err == nil {
		hd, err = ParseHeader(hb)
	}
	if err == nil && hd.Size() != fi.Size() {
		err = ErrSize
	}
	if err != nil {
		f.Close()
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			err = ErrFormat
		}
		return nil, err
	}
	return &File{f: f, h: hd}, nil
}

func (f *File) Close() error   { return f.f.Close() }
func (f *File) Header() Header { return f.h }

func (f *File) range4(i int) (Range4, error) {
	var b [size4]byte
	if _, err := f.f.ReadAt(b[:], HeaderSize+int64(i)*size4); err != nil {
		return Range4{}, err
	}
	return Range4{binary.LittleEndian.Uint32(b[:]), binary.LittleEndian.Uint32(b[4:])}, nil
}

func (f *File) range6(i int) (Range6, error) {
	var b [size6]byte
	if _, err := f.f.ReadAt(b[:], HeaderSize+int64(f.h.N4)*size4+int64(i)*size6); err != nil {
		return Range6{}, err
	}
	return Range6{U128{binary.LittleEndian.Uint64(b[:]), binary.LittleEndian.Uint64(b[8:])},
		U128{binary.LittleEndian.Uint64(b[16:]), binary.LittleEndian.Uint64(b[24:])}}, nil
}

// Contains looks addr up; it returns the matching range formatted with String.
func (f *File) Contains(addr netip.Addr) (bool, string, error) {
	if addr.Is4() {
		x := binary.BigEndian.Uint32(addr.AsSlice())
		lo, hi := 0, f.h.N4-1 // last range whose first address is <= x
		found := -1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			r, err := f.range4(mid)
			if err != nil {
				return false, "", err
			}
			if r.First <= x {
				found, lo = mid, mid+1
			} else {
				hi = mid - 1
			}
		}
		if found < 0 {
			return false, "", nil
		}
		r, err := f.range4(found)
		if err != nil || x > r.Last {
			return false, "", err
		}
		return true, r.String(), nil
	}
	x := u128(addr)
	lo, hi := 0, f.h.N6-1
	found := -1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		r, err := f.range6(mid)
		if err != nil {
			return false, "", err
		}
		if r.First.cmp(x) <= 0 {
			found, lo = mid, mid+1
		} else {
			hi = mid - 1
		}
	}
	if found < 0 {
		return false, "", nil
	}
	r, err := f.range6(found)
	if err != nil || x.cmp(r.Last) > 0 {
		return false, "", err
	}
	return true, r.String(), nil
}

// Preview returns up to n ranges (IPv4 first), formatted with String.
func (f *File) Preview(n int) ([]string, error) {
	out := []string{}
	for i := 0; i < f.h.N4 && len(out) < n; i++ {
		r, err := f.range4(i)
		if err != nil {
			return nil, err
		}
		out = append(out, r.String())
	}
	for i := 0; i < f.h.N6 && len(out) < n; i++ {
		r, err := f.range6(i)
		if err != nil {
			return nil, err
		}
		out = append(out, r.String())
	}
	return out, nil
}
