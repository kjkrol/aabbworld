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
	spans  spans
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

// wideFrom is the fewest entries a cell needs to be swept a vector at a time, when wide.
const wideFrom = 8

// Wide reports whether crowded cells are swept a vector of entries at a time.
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
	s.spans.reset(int(s.side) * int(s.side))
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
			s.spans.count(y*s.side + x)
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
	total := s.spans.place()
	if cap(s.cells) < total {
		s.cells = make([]sweepEntry, total)
	}
	s.cells = s.cells[:total]
	for i := len(s.pieces) - 1; i >= 0; i-- {
		p := &s.pieces[i]
		for y := p.y1; y <= p.y2; y++ {
			for x := p.x1; x <= p.x2; x++ {
				s.cells[s.spans.take(y*s.side+x)] = p.sweepEntry
			}
		}
	}
}

// visit tests every two entries of every cell, in the order of the cells, each pair spoken for
// by one cell only.
func (s *pairSweep) visit(fn func(a, b int32)) {
	s.seam = s.seam[:0]
	if s.spans.listed {
		for _, cell := range s.spans.touched {
			if s.spans.of[cell].n >= 2 {
				s.visitCell(cell, fn)
			}
		}
		return
	}
	for cell := range s.spans.of {
		if s.spans.of[cell].n >= 2 {
			s.visitCell(uint32(cell), fn)
		}
	}
}

// visitCell tests every two entries of a cell holding two or more.
func (s *pairSweep) visitCell(cell uint32, fn func(a, b int32)) {
	from, to := s.spans.entries(cell)
	entries := s.cells[from:to]
	cx, cy := cell%s.side, cell/s.side
	if wide && len(entries) >= wideFrom {
		s.visitWide(entries, cx, cy, fn)
		return
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

// cross takes every entry of s touching an entry of still, in the cell that owns where they
// meet, still's positions counted on from offset past all of s's; the two lay out the same cells.
func (s *pairSweep) cross(still *pairSweep, offset int32, fn func(a, b int32)) {
	for i := range s.pieces {
		p := &s.pieces[i]
		for y := p.y1; y <= p.y2; y++ {
			for x := p.x1; x <= p.x2; x++ {
				from, to := still.spans.entries(y*s.side + x)
				for k := from; k < to; k++ {
					b := &still.cells[k]
					if !p.box.Intersects(b.box) ||
						s.cellOf(max(p.box.TopLeft.X, b.box.TopLeft.X)) != x ||
						s.cellOf(max(p.box.TopLeft.Y, b.box.TopLeft.Y)) != y {
						continue
					}
					if p.multi || b.multi {
						s.seam = append(s.seam, seamPair{p.at, b.at + offset})
						continue
					}
					fn(p.at, b.at+offset)
				}
			}
		}
	}
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
