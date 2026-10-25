package spatial

import (
	"cmp"
	"slices"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
)

// pairSweep finds every two entries close enough to matter, each pair once, by position in the items.
// Its buffers are kept between calls.
type pairSweep struct {
	pieces []sweepPiece
	starts []uint32 // per cell, where its entries begin; one past the end closes the last
	cursor []uint32
	cells  []sweepEntry
	seam   []seamPair
	cols   sweepCols // a crowded cell's boxes, laid out for the wide sweep

	shift Resolution
	side  uint32

	// whole and listing are the entry in hand.
	whole   plane.AABB
	listing int32
	onImage plane.FragVisitor
}

// sweepPiece is one image of an entry's reach, and the cells it covers.
type sweepPiece struct {
	sweepEntry
	x1, y1, x2, y2 uint32
}

type sweepEntry struct {
	box geom.AABB
	at  int32
	// multi marks an entry whose reach wraps, and so is listed in more than one image.
	multi bool
	// 32 bytes in all: amd64 loads a float32 from a shifted index, not from one scaled by 8.
	_ [8]byte
}

type seamPair struct{ a, b int32 }

// wideFrom is the fewest entries a cell needs to be swept eight at a time, when wide.
const wideFrom = 8

// Wide reports whether crowded cells are swept eight entries at a time.
func Wide() bool { return wide }

// listItems gathers every matching record's grown images, and counts them into their cells.
func (s *pairSweep) listItems(
	surface *iplane.Surface, columns []column, reach float32, want Capability,
) {
	s.begin()
	first := int32(0)
	for ci := range columns {
		c := &columns[ci]
		for i := range c.n {
			if !c.caps(i).matches(want) {
				continue
			}
			at, box := first+int32(i), c.box(i)
			s.whole = *box
			surface.Expand(&s.whole, reach*min(box.Size.X, box.Size.Y))

			multi := s.whole.Overhang != (geom.Vec{})
			s.add(s.whole.AABB, at, multi)
			if multi {
				s.listing = at
				s.whole.VisitFragments(s.onImage)
			}
		}
		first += int32(c.n)
	}
}

// begin readies the sweep's buffers for a new listing.
func (s *pairSweep) begin() {
	s.pieces = s.pieces[:0]
	if s.onImage == nil {
		s.onImage = s.addImage
	}
	cells := int(s.side) * int(s.side)
	if cap(s.starts) < cells+1 {
		s.starts = make([]uint32, cells+1)
		s.cursor = make([]uint32, cells)
	}
	s.starts = s.starts[:cells+1]
	s.cursor = s.cursor[:cells]
	clear(s.starts)
}

func (s *pairSweep) addImage(_ plane.FragPosition, image geom.AABB) bool {
	s.add(image, s.listing, true)
	return true
}

func (s *pairSweep) add(box geom.AABB, at int32, multi bool) {
	p := sweepPiece{
		sweepEntry: sweepEntry{box: box, at: at, multi: multi},
		x1:         s.cellOf(box.TopLeft.X), y1: s.cellOf(box.TopLeft.Y),
		x2: s.cellOf(box.BottomRight.X), y2: s.cellOf(box.BottomRight.Y),
	}
	for y := p.y1; y <= p.y2; y++ {
		for x := p.x1; x <= p.x2; x++ {
			s.starts[y*s.side+x+1]++
		}
	}
	s.pieces = append(s.pieces, p)
}

// cellOf is the column or row holding v, held inside the grid.
func (s *pairSweep) cellOf(v float32) uint32 {
	if !(v >= 0) {
		return 0
	}
	return min(uint32(v)>>s.shift, s.side-1)
}

// sort turns the per-cell counts into offsets and deals the pieces out in listing order.
func (s *pairSweep) sort() {
	for i := 1; i < len(s.starts); i++ {
		s.starts[i] += s.starts[i-1]
	}
	copy(s.cursor, s.starts)

	total := int(s.starts[len(s.starts)-1])
	if cap(s.cells) < total {
		s.cells = make([]sweepEntry, total)
	}
	s.cells = s.cells[:total]
	for i := range s.pieces {
		p := &s.pieces[i]
		for y := p.y1; y <= p.y2; y++ {
			for x := p.x1; x <= p.x2; x++ {
				cell := y*s.side + x
				s.cells[s.cursor[cell]] = p.sweepEntry
				s.cursor[cell]++
			}
		}
	}
}

// visit tests every two entries of every cell, each pair spoken for by one cell only.
func (s *pairSweep) visit(fn func(a, b int32)) {
	s.seam = s.seam[:0]
	for cell := range s.cursor {
		entries := s.cells[s.starts[cell]:s.starts[cell+1]]
		if len(entries) < 2 {
			continue
		}
		cx, cy := uint32(cell)%s.side, uint32(cell)/s.side
		if wide && len(entries) >= wideFrom {
			s.visitWide(entries, cx, cy, fn)
			continue
		}
		for i := range entries {
			a := &entries[i]
			for j := i + 1; j < len(entries); j++ {
				b := &entries[j]
				if !a.box.Intersects(b.box) || a.at == b.at {
					continue
				}
				if s.cellOf(max(a.box.TopLeft.X, b.box.TopLeft.X)) != cx ||
					s.cellOf(max(a.box.TopLeft.Y, b.box.TopLeft.Y)) != cy {
					continue
				}
				s.meet(a, b, fn)
			}
		}
	}
}

// meet takes two entries of different items, touching in the cell that owns where they meet.
func (s *pairSweep) meet(a, b *sweepEntry, fn func(a, b int32)) {
	lo, hi := a.at, b.at
	if lo > hi {
		lo, hi = hi, lo
	}
	if a.multi || b.multi {
		s.seam = append(s.seam, seamPair{lo, hi})
		return
	}
	fn(lo, hi)
}

// flushSeam reports the pairs met through a wrapped image, each once.
func (s *pairSweep) flushSeam(fn func(a, b int32)) {
	if len(s.seam) == 0 {
		return
	}
	slices.SortFunc(s.seam, func(l, r seamPair) int {
		if c := cmp.Compare(l.a, r.a); c != 0 {
			return c
		}
		return cmp.Compare(l.b, r.b)
	})
	for i, p := range s.seam {
		if i == 0 || p != s.seam[i-1] {
			fn(p.a, p.b)
		}
	}
}
