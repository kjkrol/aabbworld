package collide

import (
	"math"
	"slices"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/aabbworld/plane"
)

// Pair is two items of a batch that may be in contact, with the flags the solver separates them by.
type Pair struct {
	A, B  int32
	Flags uint8
}

const (
	StaticA uint8 = 1 << iota
	StaticB
	Sensor
	reported
	movedA
	movedB
)

// state is what the solver remembers about a pair across passes.
type state struct {
	testedAt uint32 // the solver's clock when last measured: 0 for never, MaxUint32 for dropped
	share    float32
	flags    uint8
}

// Touch is asked once per pair, when its boxes first overlap, for the push and the part of it A
// takes; false drops the pair for the tick.
type Touch func(i int, pen geom.Vec) (geom.Vec, float32, bool)

// Solver is the state and the algorithm behind collide.Engine, working on the items of a Grid.
type Solver struct {
	pairs  []Pair
	states []state
	boxes  [][2]*plane.AABB // each pair's two boxes, where they lie

	// clock counts measurements; movedAt is its reading when each item was last pushed.
	clock   uint32
	movedAt []uint32

	// fieldAt is the clock when each item was last pushed out of the ground, fieldMoved who was.
	fieldAt    []uint32
	fieldMoved []int32
	field      fieldPass
	items      int  // how many items the batch holds, for the field's state
	fieldReady bool // the field's state is cleared for this batch
}

// Reset empties the Solver for a new batch of at most items entries, keeping the memory.
func (s *Solver) Reset(items int) {
	s.pairs = s.pairs[:0]
	s.states = s.states[:0]
	s.clock = 0
	if cap(s.movedAt) < items {
		s.movedAt = make([]uint32, items)
	}
	s.movedAt = s.movedAt[:items]
	clear(s.movedAt)
	s.items, s.fieldReady = items, false
	s.fieldMoved = s.fieldMoved[:0]
}

// readyField clears the field's state for the batch, the first time a solve meets the ground.
func (s *Solver) readyField() {
	if s.fieldReady {
		return
	}
	if cap(s.fieldAt) < s.items {
		s.fieldAt = make([]uint32, s.items)
	}
	s.fieldAt = s.fieldAt[:s.items]
	clear(s.fieldAt)
	if s.field.met == nil {
		s.field.met = map[fieldKey]uint8{}
	}
	clear(s.field.met)
	s.fieldReady = true
}

// Add enters a pair into the batch and returns its index.
func (s *Solver) Add(p Pair) int {
	s.pairs = append(s.pairs, p)
	s.states = append(s.states, state{share: 0.5, flags: p.Flags})
	return len(s.pairs) - 1
}

// Pair is the i-th pair of the batch.
func (s *Solver) Pair(i int) *Pair { return &s.pairs[i] }

// Solve reports each overlapping pair once and pushes it apart, and every movable item out of the
// field's solid ground when there is one, in up to iterations passes.
func (s *Solver) Solve(
	items []spatial.Entry, surface *iplane.Surface, iterations int,
	touch Touch, onContact func(i int, pen geom.Vec), field *Field,
) {
	pairs, states, movedAt := s.pairs, s.states[:len(s.pairs)], s.movedAt
	held := len(s.boxes)
	s.boxes = slices.Grow(s.boxes[:0], len(pairs))[:len(pairs)]
	boxes := s.boxes
	for i := range pairs {
		boxes[i] = [2]*plane.AABB{items[pairs[i].A].Box, items[pairs[i].B].Box}
	}
	if held > len(boxes) {
		clear(s.boxes[len(boxes):held])
	}
	for range iterations {
		moved := false
		for i := range pairs {
			p := &pairs[i]
			st := &states[i]

			if st.testedAt > movedAt[p.A] && st.testedAt > movedAt[p.B] {
				continue
			}
			s.clock++
			st.testedAt = s.clock

			a, b := boxes[i][0], boxes[i][1]
			pen, ok := penetration(a, b)
			if !ok {
				continue
			}

			if st.flags&reported == 0 {
				if touch != nil {
					var share float32
					if pen, share, ok = touch(i, pen); !ok {
						st.testedAt = math.MaxUint32
						continue
					}
					st.share = shareOf(share)
				}
				st.flags |= reported
				if onContact != nil {
					onContact(i, pen)
				}
			}

			if st.flags&Sensor != 0 {
				continue
			}
			pushA, pushB, ok := split(pen, st.share, st.flags&StaticA != 0, st.flags&StaticB != 0)
			if !ok {
				continue
			}
			if pushA != (geom.Vec{}) {
				surface.Translate(a, pushA)
				movedAt[p.A] = s.clock
				st.flags |= movedA
				moved = true
			}
			if pushB != (geom.Vec{}) {
				surface.Translate(b, pushB)
				movedAt[p.B] = s.clock
				st.flags |= movedB
				moved = true
			}
		}
		if field != nil && s.pushOutOfGround(items, surface, field) {
			moved = true
		}
		if !moved {
			return
		}
	}
}

// pushOutOfGround pushes every movable item that moved since it last met the ground out of it, and
// reports whether any was pushed.
func (s *Solver) pushOutOfGround(
	items []spatial.Entry, surface *iplane.Surface, field *Field,
) bool {
	s.readyField()
	p := &s.field
	if p.visit == nil {
		p.visit, p.wall = p.meet, p.meetWall
	}
	p.field, p.surface = field, surface
	any := false
	for i := range items {
		it := &items[i]
		if !movable(it) || s.fieldAt[i] > s.movedAt[i] {
			continue
		}
		s.clock++
		s.fieldAt[i] = s.clock
		p.item, p.it, p.sensor, p.pushed = int32(i), it, it.Caps&spatial.Sensor != 0, false
		box := geom.AABB{TopLeft: it.Box.TopLeft, BottomRight: it.Box.TopLeft.Add(it.Box.Size)}
		field.Solid.Solid(it.ID, box, p.visit)
		if field.Walls != nil {
			box = geom.AABB{TopLeft: it.Box.TopLeft, BottomRight: it.Box.TopLeft.Add(it.Box.Size)}
			field.Walls.Walls(it.ID, box, p.wall)
		}
		if p.pushed {
			s.movedAt[i] = s.clock
			s.fieldMoved = append(s.fieldMoved, int32(i))
			any = true
		}
	}
	return any
}

// VisitMoved calls fn for every item the solver pushed, once per pair it was pushed in.
func (s *Solver) VisitMoved(fn func(item int32)) {
	for i := range s.states {
		f := s.states[i].flags
		if f&movedA != 0 {
			fn(s.pairs[i].A)
		}
		if f&movedB != 0 {
			fn(s.pairs[i].B)
		}
	}
	for _, i := range s.fieldMoved {
		fn(i)
	}
}

// penetration is how far a and b interpenetrate, wrapped images included; false when apart, or
// when they overlap by no more than float32's rounding of where they lie: half of such an overlap
// moves neither box, so shared between them it would only wander round a stack, never gone.
func penetration(a, b *plane.AABB) (geom.Vec, bool) {
	if a.Overhang == (geom.Vec{}) && b.Overhang == (geom.Vec{}) {
		pen := a.AABB.Penetration(b.AABB)
		return pen, pen != geom.Vec{} && !rounding(pen, a.AABB, b.AABB)
	}
	hit, ok := a.DeepestOverlapWith(b)
	return hit.Penetration, ok && !rounding(hit.Penetration, hit.BoxA, hit.BoxB)
}

// rounding reports whether pen, along one axis, lies within a few float32 steps of the boxes'
// edges on that axis
func rounding(pen geom.Vec, a, b geom.AABB) bool {
	if pen.X != 0 {
		far := max(a.BottomRight.X, b.BottomRight.X, -a.TopLeft.X, -b.TopLeft.X)
		return max(pen.X, -pen.X) <= far*0x1p-22
	}
	far := max(a.BottomRight.Y, b.BottomRight.Y, -a.TopLeft.Y, -b.TopLeft.Y)
	return max(pen.Y, -pen.Y) <= far*0x1p-22
}

// split shares a penetration between the sides that can move, A taking share of it where both
// can; ok is false when neither can.
func split(pen geom.Vec, share float32, staticA, staticB bool) (pushA, pushB geom.Vec, ok bool) {
	switch {
	case staticA && staticB:
		return geom.Vec{}, geom.Vec{}, false
	case staticB:
		return pen, geom.Vec{}, true
	case staticA:
		return geom.Vec{}, negate(pen), true
	default:
		rest := 1 - share
		return geom.NewVec(pen.X*share, pen.Y*share), geom.NewVec(-pen.X*rest, -pen.Y*rest), true
	}
}

// shareOf holds a handler's share within 0 and 1, a half for one that is no number
func shareOf(share float32) float32 {
	if share != share {
		return 0.5
	}
	return min(max(share, 0), 1)
}

func negate(v geom.Vec) geom.Vec { return geom.NewVec(-v.X, -v.Y) }
