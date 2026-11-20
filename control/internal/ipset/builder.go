package ipset

import (
	"cmp"
	"container/heap"
	"encoding/binary"
	"net/netip"
	"slices"
)

// Builder collects networks and produces their merged ranges, sorted. Networks are kept
// in fixed-size chunks (growing never copies them); the chunks are sorted one by one and
// merged on the fly, so ranges can be written to a file while the chunks are released
// (Dir.WriteBuilder): building a list costs about 8 bytes per IPv4 network (32 per IPv6
// network) once.
type Builder struct {
	v4 chunks[Range4]
	v6 chunks[Range6]
}

const (
	chunk4 = 1 << 16 // 512 KiB
	chunk6 = 1 << 14 // 512 KiB
)

// Add adds a network.
func (b *Builder) Add(p netip.Prefix) {
	a := p.Masked().Addr()
	if a.Is4() {
		first := binary.BigEndian.Uint32(a.AsSlice())
		host := uint32(0)
		if p.Bits() < 32 {
			host = ^uint32(0) >> p.Bits()
		}
		b.v4.add(Range4{first, first | host}, chunk4)
		return
	}
	first := u128(a)
	last := first
	switch n := p.Bits(); {
	case n == 0:
		last = U128{^uint64(0), ^uint64(0)}
	case n < 64:
		last = U128{first.Hi | ^uint64(0)>>n, ^uint64(0)}
	case n < 128:
		last.Lo = first.Lo | ^uint64(0)>>(n-64)
	}
	b.v6.add(Range6{first, last}, chunk6)
}

// AddRange4 / AddRange6 add an inclusive range (first <= last).
func (b *Builder) AddRange4(r Range4) { b.v4.add(r, chunk4) }
func (b *Builder) AddRange6(r Range6) { b.v6.add(r, chunk6) }

// Len is the number of networks added so far.
func (b *Builder) Len() int { return b.v4.len() + b.v6.len() }

// Set returns the merged set. The builder is emptied.
func (b *Builder) Set() *Set {
	s := &Set{}
	s.Networks = b.ranges(func(r Range4) error {
		s.V4 = append(s.V4, r)
		return nil
	}, func(r Range6) error {
		s.V6 = append(s.V6, r)
		return nil
	})
	return s
}

// ranges calls f4 for the merged IPv4 ranges in order, then f6 for the IPv6 ones (until
// one returns an error), and empties the builder. Overlapping and adjacent networks
// become one range. It returns the number of distinct networks.
func (b *Builder) ranges(f4 func(Range4) error, f6 func(Range6) error) (distinct int) {
	var cur4 Range4
	have := false
	var err error
	distinct += b.v4.sorted(cmp4, func(r Range4) bool {
		if have && (cur4.Last == ^uint32(0) || r.First <= cur4.Last+1) {
			cur4.Last = max(cur4.Last, r.Last)
			return true
		}
		if have {
			err = f4(cur4)
		}
		cur4, have = r, true
		return err == nil
	})
	if have && err == nil {
		err = f4(cur4)
	}
	if err != nil {
		b.v6 = chunks[Range6]{}
		return distinct
	}
	var cur6 Range6
	have = false
	distinct += b.v6.sorted(cmp6, func(r Range6) bool {
		if have {
			next, over := cur6.Last.inc()
			if over || r.First.cmp(next) <= 0 {
				if r.Last.cmp(cur6.Last) > 0 {
					cur6.Last = r.Last
				}
				return true
			}
			err = f6(cur6)
		}
		cur6, have = r, true
		return err == nil
	})
	if have && err == nil {
		_ = f6(cur6)
	}
	return distinct
}

func cmp4(a, b Range4) int {
	if c := cmp.Compare(a.First, b.First); c != 0 {
		return c
	}
	return cmp.Compare(a.Last, b.Last)
}

func cmp6(a, b Range6) int {
	if c := a.First.cmp(b.First); c != 0 {
		return c
	}
	return a.Last.cmp(b.Last)
}

// chunks is a list of slices of at most size elements each.
type chunks[T comparable] struct{ c [][]T }

func (k *chunks[T]) add(x T, size int) {
	n := len(k.c)
	if n == 0 || len(k.c[n-1]) >= size {
		k.c = append(k.c, nil) // the first chunk grows from small; later ones are full size
		if n > 0 {
			k.c[n] = make([]T, 0, size)
		}
		n++
	}
	k.c[n-1] = append(k.c[n-1], x)
}

func (k *chunks[T]) len() int {
	n := 0
	for _, c := range k.c {
		n += len(c)
	}
	return n
}

// sorted sorts every chunk and calls fn for all elements in order (a k-way merge) until
// fn returns false. Chunks are released as they are consumed. It returns the number of
// distinct elements seen.
func (k *chunks[T]) sorted(less func(a, b T) int, fn func(T) bool) (distinct int) {
	h := &mergeHeap[T]{cmp: less}
	for _, c := range k.c {
		if len(c) > 0 {
			slices.SortFunc(c, less)
			h.s = append(h.s, c)
		}
	}
	k.c = nil
	heap.Init(h)
	var prev T
	first := true
	for len(h.s) > 0 {
		x := h.s[0][0]
		if h.s[0] = h.s[0][1:]; len(h.s[0]) == 0 {
			heap.Pop(h)
		} else {
			heap.Fix(h, 0)
		}
		if first || x != prev {
			distinct++
		}
		prev, first = x, false
		if !fn(x) {
			break
		}
	}
	return distinct
}

// mergeHeap orders slices by their first element.
type mergeHeap[T any] struct {
	s   [][]T
	cmp func(a, b T) int
}

func (h *mergeHeap[T]) Len() int           { return len(h.s) }
func (h *mergeHeap[T]) Less(i, j int) bool { return h.cmp(h.s[i][0], h.s[j][0]) < 0 }
func (h *mergeHeap[T]) Swap(i, j int)      { h.s[i], h.s[j] = h.s[j], h.s[i] }
func (h *mergeHeap[T]) Push(x any)         { h.s = append(h.s, x.([]T)) }
func (h *mergeHeap[T]) Pop() any {
	n := len(h.s) - 1
	x := h.s[n]
	h.s[n] = nil
	h.s = h.s[:n]
	return x
}
