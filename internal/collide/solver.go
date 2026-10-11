package collide

import (
	"math"

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
	flags    uint8
}

// Touch is asked once per pair, when its boxes first overlap; false drops the pair for the tick.
type Touch func(i int, pen geom.Vec) (geom.Vec, bool)

// Solver is the state and the algorithm behind collide.Engine, working on the items of a Grid.
type Solver struct {
	pairs  []Pair
	states []state

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
	s.states = append(s.states, state{flags: p.Flags})
	return len(s.pairs) - 1
}

// Pair is the i-th pair of the batch.
func (s *Solver) Pair(i int) *Pair { return &s.pairs[i] }

// Solve reports each overlapping pair once and pushes it apart, and every movable item out of the
// field's solid ground when there is one, in up to iterations passes.
func (s *Solver) Solve(items []spatial.Item, surface *iplane.Surface, iterations int, touch Touch, onContact func(i int, pen geom.Vec), field *Field) {
	pairs, states, movedAt := s.pairs, s.states[:len(s.pairs)], s.movedAt
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

			a, b := &items[p.A].Box, &items[p.B].Box
			pen, ok := penetration(a, b)
			if !ok {
				continue
			}

			if st.flags&reported == 0 {
				if touch != nil {
					if pen, ok = touch(i, pen); !ok {
						st.testedAt = math.MaxUint32
						continue
					}
				}
				st.flags |= reported
				if onContact != nil {
					onContact(i, pen)
				}
			}

			if st.flags&Sensor != 0 {
				continue
			}
			pushA, pushB, ok := split(pen, st.flags&StaticA != 0, st.flags&StaticB != 0)
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
func (s *Solver) pushOutOfGround(items []spatial.Item, surface *iplane.Surface, field *Field) bool {
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

// penetration is how far a and b interpenetrate, wrapped images included; false when apart.
func penetration(a, b *plane.AABB) (geom.Vec, bool) {
	if a.Overhang == (geom.Vec{}) && b.Overhang == (geom.Vec{}) {
		pen := a.AABB.Penetration(b.AABB)
		return pen, pen != geom.Vec{}
	}
	hit, ok := a.DeepestOverlapWith(b)
	return hit.Penetration, ok
}

// split shares a penetration between the sides that can move; ok is false when neither can.
func split(pen geom.Vec, staticA, staticB bool) (pushA, pushB geom.Vec, ok bool) {
	switch {
	case staticA && staticB:
		return geom.Vec{}, geom.Vec{}, false
	case staticB:
		return pen, geom.Vec{}, true
	case staticA:
		return geom.Vec{}, negate(pen), true
	default:
		half := geom.NewVec(pen.X/2, pen.Y/2)
		return half, negate(half), true
	}
}

func negate(v geom.Vec) geom.Vec { return geom.NewVec(-v.X, -v.Y) }
